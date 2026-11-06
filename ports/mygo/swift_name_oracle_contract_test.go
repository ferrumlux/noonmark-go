// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func swiftOracleContractRoot(base, repository string) (string, error) {
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return "", fmt.Errorf("NOONMARK_MYGO_BUILD_ROOT must be an explicit absolute clean external directory; use scripts/check")
	}
	root, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("oracle contract build root is not a directory")
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return "", err
	}
	for _, pair := range [][2]string{{root, repository}, {repository, root}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil {
			return "", err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			return "", fmt.Errorf("oracle contract build root overlaps source repository")
		}
	}
	return root, nil
}

// This default scripts/check gate runs offline Python contracts, never Swift.
// The actual macOS compilation and capture remain a separate hosted symptom gate.
func TestSwiftNameOracleCompilationContract(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root, err := swiftOracleContractRoot(os.Getenv("NOONMARK_MYGO_BUILD_ROOT"), repository)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "-B", "scripts/swift-name-oracle/test_oracle.py")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "RUNNER_TEMP="+root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Swift oracle offline compilation contract failed: %v\n%s", err, output)
	}
	t.Logf("Offline contracts only, not Swift execution:\n%s", output)
}

func TestSwiftNameOracleContractBuildRootGuard(t *testing.T) {
	base := t.TempDir()
	repository := filepath.Join(base, "repo")
	external := filepath.Join(base, "build")
	inside := filepath.Join(repository, "build")
	for _, dir := range []string{repository, external, inside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("synthetic-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "relative", filepath.Join(base, "missing"), file, repository, inside, base, external + string(os.PathSeparator)} {
		if _, err := swiftOracleContractRoot(path, repository); err == nil {
			t.Fatalf("accepted unsafe build root %q", path)
		}
	}
	physical, err := filepath.EvalSymlinks(external)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := swiftOracleContractRoot(external, repository); err != nil || got != physical {
		t.Fatalf("external build root rejected: %q, %v", got, err)
	}
	link := filepath.Join(base, "source-link")
	if err := os.Symlink(repository, link); err != nil {
		t.Fatal(err)
	}
	if _, err := swiftOracleContractRoot(link, repository); err == nil {
		t.Fatal("accepted symlink into source repository")
	}
}
