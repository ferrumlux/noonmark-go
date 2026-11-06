// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run a fresh standalone checkout, with no assumed sibling repository, for both the main suite and the independent regressions. These fixtures are
// synthetic and never certify an actual Swift execution. The Python entry
// validates source/output boundaries before creating its exclusive evidence
// runs, including the immutable source snapshots used by the real Go compiler.
func TestSwiftNameComparisonRegressionGate(t *testing.T) {
	base := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		t.Fatal("NOONMARK_MYGO_BUILD_ROOT must be an explicit absolute clean external path; use scripts/check")
	}
	goName := os.Getenv("NOONMARK_GO")
	if goName == "" {
		goName = "go"
	}
	goPath, err := exec.LookPath(goName)
	if err != nil {
		t.Fatal(err)
	}
	goPath, err = filepath.EvalSymlinks(goPath)
	if err != nil {
		t.Fatal(err)
	}
	goPath, err = filepath.Abs(goPath)
	if err != nil {
		t.Fatal(err)
	}
	// Do not mkdir here: the verified Python guard must reject unsafe paths
	// before any writes. Existing runs are never reused or overwritten.
	evidence := filepath.Join(base, "swift-name-comparison-tests")
	cmd := exec.Command("python3", "-B", "scripts/swift-name-compare/run-tests.py")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "GO="+goPath, "NOONMARK_SWIFT_COMPARE_TEST_ROOT="+evidence)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Swift comparison tool regression gate failed: %v\n%s", err, output)
	}
	t.Logf("%s", output)
	t.Logf("Synthetic-only evidence is retained outside the source tree: %s", evidence)
}
