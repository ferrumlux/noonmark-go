// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
)

// Error propagation unit tests; real Service.Close and lane observation require
// the separate bounded runtime fault experiment, not this synthetic callback.
func TestRuntimeExitCleanupFailureCannotReportSuccess(t *testing.T) {
	for _, runFailed := range []bool{false, true} {
		for _, closeFailed := range []bool{false, true} {
			var runErr, closeErr error
			if runFailed {
				runErr = errors.New("event loop sentinel")
			}
			if closeFailed {
				closeErr = errors.New("service close sentinel")
			}
			var output bytes.Buffer
			old := log.Writer()
			log.SetOutput(&output)
			calls := 0
			code := runtimeExitStatus(runErr, func() error { calls++; return closeErr })
			log.SetOutput(old)
			want := 0
			if runFailed || closeFailed {
				want = 1
			}
			if calls != 1 || code != want {
				t.Fatalf("run=%v close=%v calls=%d code=%d", runFailed, closeFailed, calls, code)
			}
			if runFailed && !strings.Contains(output.String(), runErr.Error()) {
				t.Fatal("lost run failure")
			}
			if closeFailed && !strings.Contains(output.String(), closeErr.Error()) {
				t.Fatal("lost close failure")
			}
		}
	}
}
func TestRuntimeExitMainHasSingleClosePerPath(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	// Verify the narrowly scoped, terminal after-open business failure owner
	// before reconstructing and checking the original three ownership paths.
	prior, err := runtimeAdmissionLegacyMainSource(string(b))
	if err != nil {
		t.Fatal(err)
	}
	s, err := runtimeExitLegacyOwnershipSource(prior)
	if err != nil {
		t.Fatal(err)
	}
	s = s[strings.Index(s, "func main()"):]
	if strings.Contains(s, "defer service.Close()") {
		t.Fatal("deferred second close may hide original error")
	}
	if strings.Count(s, "service.Close()") != 3 {
		t.Fatal("unexpected Service ownership paths")
	}
	// Each mutually exclusive owner path is explicit: render, failed enable, native.
	for _, fragment := range []string{"renderErr = errors.Join(renderErr, service.Close())", "closeErr := service.Close()", "errors.Join(err, closeErr)", "runtimeExitStatus(runErr, func() error {", "return err", "cleanupErr := cleanup()", "errors.Join(runErr, cleanupErr)"} {
		if !strings.Contains(s, fragment) {
			t.Fatal("missing propagation", fragment)
		}
	}
	closeAt := strings.LastIndex(s, "err := service.Close()")
	finalAt := strings.Index(s, "payload, err := probe.Finalize()")
	exitAt := strings.Index(s, "os.Exit(code)")
	if closeAt < 0 || finalAt < closeAt || exitAt < finalAt {
		t.Fatal("exit before cleanup/report")
	}
}
