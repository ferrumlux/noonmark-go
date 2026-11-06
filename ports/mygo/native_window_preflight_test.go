package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func smokeArgs(parent string) []string {
	return []string{"-init-session", filepath.Join(parent, "session"), "-native-window-smoke", filepath.Join(parent, "stage1.json")}
}

func TestNativeWindowPreflightRejectsBeforeWrites(t *testing.T) {
	cases := [][]string{
		{"-list=false"}, {"-prototype=false"}, {"-render="}, {"-render-one="}, {"-unknown=1"},
		{"--"}, {"positional"}, {"-screen="}, {"-screen", "pool"}, {"-screen", "day", "--screen=day"},
		{"-native-window-smoke="}, {"-init-session="}, {"-session=/unopened/session"},
	}
	for _, extra := range cases {
		t.Run(extra[0], func(t *testing.T) {
			parent := t.TempDir()
			args := append(smokeArgs(parent), extra...)
			if report, err := nativeWindowPreflight(args); err == nil || report != nil {
				t.Fatalf("accepted %q", args)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatalf("preflight wrote files: %v %v", entries, err)
			}
		})
	}
	for _, args := range [][]string{{"-native-window-smoke"}, {"--native-window-smoke="}, {"-native-window-smoke", "x"}, {"-native-window-smoke=x", "-init-session=y"}} {
		if _, err := nativeWindowPreflight(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}

func TestNativeWindowPreflightNonSmokeUnchanged(t *testing.T) {
	for _, args := range [][]string{nil, {"-list=false"}, {"-unknown"}, {"--", "anything"}, {"-render-one="}, {"-native-window-smoke-other=anything"}} {
		report, err := nativeWindowPreflight(args)
		if report != nil || err != nil {
			t.Fatalf("changed non-smoke branch: %q", args)
		}
	}
}

func TestNativeWindowPreflightFixedPaths(t *testing.T) {
	for _, kind := range []string{"relative", "traversal", "report-name", "session-name", "report-parent", "existing-file", "existing-dir", "existing-session", "missing-open"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			args := smokeArgs(parent)
			switch kind {
			case "relative":
				args[1] = "session"
			case "traversal":
				args[1] = parent + string(os.PathSeparator) + "x" + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "session"
			case "report-name":
				args[3] = filepath.Join(parent, "stage2.json")
			case "session-name":
				args[1] = filepath.Join(parent, "other")
			case "report-parent":
				args[3] = filepath.Join(parent, "session", "stage1.json")
			case "existing-file":
				if err := os.WriteFile(args[3], []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			case "existing-dir":
				if err := os.Mkdir(args[3], 0700); err != nil {
					t.Fatal(err)
				}
			case "existing-session":
				if err := os.Mkdir(args[1], 0700); err != nil {
					t.Fatal(err)
				}
			case "missing-open":
				args[0] = "-session"
				args[3] = filepath.Join(parent, "stage2.json")
			}
			if _, err := nativeWindowPreflight(args); err == nil {
				t.Fatal("accepted invalid path")
			}
			if kind == "existing-file" {
				b, err := os.ReadFile(args[3])
				if err != nil || string(b) != "original" {
					t.Fatal("overwrote existing file")
				}
			}
		})
	}
}

func TestNativeWindowPreflightLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs host privilege; reparse rejection requires Windows host test")
	}
	for _, kind := range []string{"parent", "report", "session"} {
		t.Run(kind, func(t *testing.T) {
			parent := t.TempDir()
			target := t.TempDir()
			args := smokeArgs(parent)
			switch kind {
			case "parent":
				link := filepath.Join(parent, "linked")
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
				args = smokeArgs(link)
			case "report":
				if err := os.Symlink(filepath.Join(target, "new"), args[3]); err != nil {
					t.Fatal(err)
				}
			case "session":
				if err := os.Symlink(target, args[1]); err != nil {
					t.Fatal(err)
				}
				args[0] = "-session"
				args[3] = filepath.Join(parent, "stage2.json")
			}
			if _, err := nativeWindowPreflight(args); err == nil {
				t.Fatal("accepted link")
			}
			entries, _ := os.ReadDir(target)
			if len(entries) != 0 {
				t.Fatal("wrote through link")
			}
		})
	}
}

func TestNativeWindowPreflightCreateOpenAndCollision(t *testing.T) {
	parent := t.TempDir()
	args := smokeArgs(parent)
	r, err := nativeWindowPreflight(args)
	if err != nil {
		t.Fatal(err)
	}
	if r.stage != "create" {
		t.Fatal(r.stage)
	}
	if err = r.WriteFinal([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	if err = r.WriteFinal([]byte("{}\n")); err == nil {
		t.Fatal("finalized twice")
	}
	if _, err = nativeWindowPreflight(args); err == nil {
		t.Fatal("accepted collision")
	}
	if err = os.Mkdir(args[1], 0700); err != nil {
		t.Fatal(err)
	}
	r, err = nativeWindowPreflight([]string{"--session=" + args[1], "--native-window-smoke=" + filepath.Join(parent, "stage2.json"), "--screen=day"})
	if err != nil {
		t.Fatal(err)
	}
	if r.stage != "open" {
		t.Fatal(r.stage)
	}
	if err = r.WriteFinal([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
}

func TestNativeWindowPreflightExclusiveRace(t *testing.T) {
	parent := t.TempDir()
	args := smokeArgs(parent)
	var wg sync.WaitGroup
	success := make(chan *nativeWindowReportFile, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := nativeWindowPreflight(args)
			if err == nil {
				success <- r
			}
		}()
	}
	wg.Wait()
	close(success)
	n := 0
	for r := range success {
		n++
		if err := r.WriteFinal([]byte("{}\n")); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("exclusive winners %d", n)
	}
}

type smokeReportSink struct {
	short                       bool
	writeErr, syncErr, closeErr error
	writes, syncs, closes       int
}

func (s *smokeReportSink) Write(p []byte) (int, error) {
	s.writes++
	if s.short {
		return len(p) - 1, nil
	}
	return len(p), s.writeErr
}
func (s *smokeReportSink) Sync() error  { s.syncs++; return s.syncErr }
func (s *smokeReportSink) Close() error { s.closes++; return s.closeErr }
func TestNativeWindowReportIOFailures(t *testing.T) {
	sentinel := errors.New("injected")
	for _, tc := range []struct {
		name string
		s    smokeReportSink
		want error
	}{{"short", smokeReportSink{short: true}, io.ErrShortWrite}, {"write", smokeReportSink{writeErr: sentinel}, sentinel}, {"sync", smokeReportSink{syncErr: sentinel}, sentinel}, {"close", smokeReportSink{closeErr: sentinel}, sentinel}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := nativeWindowWriteReport(&tc.s, []byte("{}\n")); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if tc.s.closes != 1 {
				t.Fatal("not closed")
			}
		})
	}
	for _, p := range [][]byte{nil, make([]byte, 65537)} {
		s := &smokeReportSink{}
		if nativeWindowWriteReport(s, p) == nil || s.writes != 0 || s.closes != 1 {
			t.Fatal("invalid size accepted")
		}
	}
	s := &smokeReportSink{}
	if err := nativeWindowWriteReport(s, []byte("{}\n")); err != nil || s.writes != 1 || s.syncs != 1 || s.closes != 1 {
		t.Fatal("success did not perform all IO", err)
	}
}
