// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type yearCommandResult struct {
	output                []byte
	err                   error
	startedPID, waitedPID int
}

// This waits for the direct child, including after cancellation. It does not
// claim that killing a go build driver kills its compiler descendants.
func runYearCommand(ctx context.Context, executable string, args, env []string) yearCommandResult {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = env
	cmd.WaitDelay = 2 * time.Second
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	result := yearCommandResult{output: output.Bytes(), err: err}
	if cmd.Process != nil {
		result.startedPID = cmd.Process.Pid
	}
	if cmd.ProcessState != nil {
		result.waitedPID = cmd.ProcessState.Pid()
	}
	if ctx.Err() != nil {
		result.err = errors.Join(ctx.Err(), err)
	}
	return result
}

func yearProcessDeadline(now, parent time.Time, hasParent bool) (time.Time, error) {
	deadline := now.Add(8 * time.Minute)
	if hasParent && parent.Add(-15*time.Second).Before(deadline) {
		deadline = parent.Add(-15 * time.Second)
	}
	if deadline.Sub(now) <= 2*time.Second {
		return time.Time{}, errors.New("insufficient parent test budget for fixture process and cleanup")
	}
	return deadline, nil
}

func buildYearFixture(t *testing.T, session string) {
	t.Helper()
	parent, ok := t.Deadline()
	deadline, err := yearProcessDeadline(time.Now(), parent, ok)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	binary := filepath.Join(filepath.Dir(session), "carryover-fixture.test")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if _, err := os.Lstat(binary); !os.IsNotExist(err) {
		t.Fatal("fixture binary must have a fresh path", err)
	}
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	args := []string{"test", "-c", "-p=1", "-buildvcs=false", "-o", binary, "./internal/taskapp"}
	build := runYearCommand(ctx, goTool, args, os.Environ())
	t.Logf("fixture build: %s %s\n%s", goTool, strings.Join(args, " "), build.output)
	if build.err != nil {
		t.Fatal("fixture build failed; no session process started", build.err)
	}
	// The fixture's own timeout expires before cancellation, which in turn
	// expires before the parent test. The same deadline covers both stages.
	timeout := time.Until(deadline) - 2*time.Second
	if timeout <= 0 || ctx.Err() != nil {
		t.Fatal("fixture execution budget expired before start")
	}
	args = []string{"-test.run=^TestCarryoverYearFixture$", "-test.count=1", "-test.v", "-test.timeout=" + timeout.String()}
	env := append(os.Environ(), "NOONMARK_CARRYOVER_YEAR_SESSION="+session)
	run := runYearCommand(ctx, binary, args, env)
	t.Logf("fixture direct process pid=%d waited_pid=%d: %s %s\n%s", run.startedPID, run.waitedPID, binary, strings.Join(args, " "), run.output)
	if run.err != nil {
		t.Fatal("real domain fixture process failed", run.err)
	}
	if run.startedPID == 0 || run.waitedPID != run.startedPID {
		t.Fatal("fixture child was not synchronously reaped")
	}
}

func TestCarryoverYearProcessBudget(t *testing.T) {
	now := time.Unix(1000, 0)
	if got, err := yearProcessDeadline(now, time.Time{}, false); err != nil || got != now.Add(8*time.Minute) {
		t.Fatal("missing deadline must still have a fixed bound", got, err)
	}
	if got, err := yearProcessDeadline(now, now.Add(time.Minute), true); err != nil || got != now.Add(45*time.Second) {
		t.Fatal("parent cleanup margin not preserved", got, err)
	}
	for _, remaining := range []time.Duration{-time.Second, 0, 15 * time.Second, 17 * time.Second} {
		if _, err := yearProcessDeadline(now, now.Add(remaining), true); err == nil {
			t.Fatal("insufficient budget must fail before starting child", remaining)
		}
	}
}

// A selected helper invocation only uses its own freshly allocated probe path.
// In the ordinary suite this test returns normally, without Skip or side effects.
func TestCarryoverYearProcessHelper(t *testing.T) {
	mode := os.Getenv("NOONMARK_YEAR_PROCESS_PROBE")
	if mode == "" {
		return
	}
	path := os.Getenv("NOONMARK_YEAR_PROCESS_PROBE_PATH")
	if !filepath.IsAbs(path) {
		t.Fatal("fresh absolute process probe path required")
	}
	switch mode {
	case "success":
		fmt.Fprint(os.Stdout, "fixture probe success")
	case "failure":
		t.Fatal("intentional fixture process failure")
	case "block":
		for n := 0; ; n++ {
			if err := os.WriteFile(path, []byte(fmt.Sprint(n)), 0600); err != nil {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	default:
		t.Fatal("unknown process probe mode")
	}
}

func TestCarryoverYearDirectProcessLifecycle(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-test.run=^TestCarryoverYearProcessHelper$", "-test.timeout=30s"}
	for _, mode := range []string{"success", "failure", "cancel", "deadline", "already_cancelled"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "probe")
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			helperMode := mode
			if mode == "cancel" || mode == "deadline" || mode == "already_cancelled" {
				helperMode = "block"
			}
			if mode == "deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 5*time.Second)
				defer stop()
			}
			if mode == "already_cancelled" {
				cancel()
			}
			env := append(os.Environ(), "NOONMARK_YEAR_PROCESS_PROBE="+helperMode, "NOONMARK_YEAR_PROCESS_PROBE_PATH="+path)
			done := make(chan yearCommandResult, 1)
			go func() { done <- runYearCommand(ctx, executable, args, env) }()
			if mode == "cancel" || mode == "deadline" {
				ready := time.NewTimer(10 * time.Second)
				defer ready.Stop()
				for {
					if _, err := os.Stat(path); err == nil {
						break
					}
					select {
					case result := <-done:
						t.Fatalf("child exited before becoming active: %v %s", result.err, result.output)
					case <-ready.C:
						cancel()
						<-done
						t.Fatal("probe never became active")
					case <-time.After(10 * time.Millisecond):
					}
				}
				if mode == "cancel" {
					cancel()
				}
			}
			var result yearCommandResult
			if mode == "cancel" || mode == "deadline" {
				<-ctx.Done()
				select {
				case result = <-done:
				case <-time.After(3 * time.Second):
					// Keep waiting for the helper's independent 30-second test
					// timeout so even a broken cancellation runner is reaped.
					t.Error("direct child was not reaped within cancellation and pipe-close bound")
					result = <-done
				}
			} else {
				result = <-done
			}
			if mode == "already_cancelled" {
				if !errors.Is(result.err, context.Canceled) || result.startedPID != 0 || result.waitedPID != 0 {
					t.Fatal("cancelled context started a child", result)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("cancelled context wrote a probe", err)
				}
				return
			}
			if result.startedPID == 0 || result.waitedPID != result.startedPID {
				t.Fatal("direct child Wait did not reap the started PID", result)
			}
			switch mode {
			case "success":
				if result.err != nil || !bytes.Contains(result.output, []byte("fixture probe success")) {
					t.Fatal("successful output lost", result)
				}
			case "failure":
				if result.err == nil || !bytes.Contains(result.output, []byte("intentional fixture process failure")) {
					t.Fatal("nonzero fixture result hidden", result)
				}
			case "cancel", "deadline":
				want := context.Canceled
				if mode == "deadline" {
					want = context.DeadlineExceeded
				}
				if !errors.Is(result.err, want) {
					t.Fatal("wrong cancellation result", result.err)
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(50 * time.Millisecond)
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("probe continued writing after direct child was reaped", err)
				}
				t.Logf("cancelled process pid=%d synchronously waited=%d; probe unchanged", result.startedPID, result.waitedPID)
			}
		})
	}
}
