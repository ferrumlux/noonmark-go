// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailureGateRegistry(t *testing.T) {
	b, e := os.ReadFile("docs/engineering/failure-cases/gates.tsv")
	if e != nil {
		t.Fatal(e)
	}
	funcs := map[string]bool{}
	for _, dir := range []string{".", "internal/taskapp", "internal/taskview", "internal/inputdraft", "internal/titlesession", "internal/titlecoordinator", "domain-core/core", "domain-core/textcanon", "domain-extensions/classification", "scripts/swift-snapshot-oracle/go-probe"} {
		files, e := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if e != nil {
			t.Fatal(e)
		}
		for _, path := range files {
			f, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if e != nil {
				t.Fatal(e)
			}
			for _, decl := range f.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					funcs[fn.Name.Name] = true
				}
			}
		}
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) < 2 {
		t.Fatal("failure case has no gates")
	}
	for _, line := range lines[1:] {
		parts := strings.Split(line, "\t")
		if len(parts) != 5 || parts[3] != "scripts/check" {
			t.Fatal("invalid gate registration")
		}
		for _, col := range parts[1:3] {
			for _, name := range strings.Split(col, ";") {
				if !funcs[name] {
					t.Fatalf("missing regression gate %s", name)
				}
			}
		}
	}
	script, e := os.ReadFile("scripts/check")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(script), `"$GO" test -p=1 -race -buildvcs=false ./...`) {
		t.Fatal("aggregate no longer runs registered package tests")
	}
}
