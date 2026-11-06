// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runCanonical16Python(t *testing.T, script string, args ...string) {
	t.Helper()
	path := filepath.Join("domain-extensions", "classification", "canonical16", script)
	argv := append([]string{"-B", path}, args...)
	cmd := exec.Command("python3", argv...)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("canonical16 verification failed: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

// Exercise the real shell verification entry against synthetic invalid paths.
// It must reject every input before creating output or invoking Go. The Python
// suite also verifies that two valid invocations allocate separate 0700 runs
// without truncating the first run's evidence. No production data is accessed.
func TestCanonical16RejectsUnsafeVerificationPaths(t *testing.T) {
	runCanonical16Python(t, "scripts/test_guard_paths.py", "--scratch", t.TempDir())
}

func TestCanonical16RegeneratesPinnedTables(t *testing.T) {
	runCanonical16Python(t, "internal/generate/generate.py", "--check")
}
