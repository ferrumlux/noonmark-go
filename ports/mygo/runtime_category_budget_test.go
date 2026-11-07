// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
)

func TestCategoryBudgetOversizeDraftRejectedBeforeQueue(t *testing.T) {
	f := newCategoryFixture(t, true)
	e := categoryBegin(t, f, "")
	raw := strings.Repeat("x", ordinary.DefaultLimits().MaxBytes/3+1)
	e.native.draft = raw
	e.savedUntil = f.clock.now.Add(time.Second)
	before, err := f.svc.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	f.s.submitCategory(e, ordinary.Choice{Kind: ordinary.ChoiceNew})
	after, err := f.svc.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(e.failure, ordinary.ErrResourceLimit) || e.pending || len(f.posts.q) != 0 || len(f.io.q) != 0 || !e.savedUntil.IsZero() || !e.creating || e.native.draft != raw || !reflect.DeepEqual(before, after) {
		t.Fatalf("UI preprocessing limit did not reject before queue: failure=%v pending=%v posts=%d lane=%d saved=%v retained=%v viewSame=%v", e.failure, e.pending, len(f.posts.q), len(f.io.q), !e.savedUntil.IsZero(), e.native.draft == raw, reflect.DeepEqual(before, after))
	}
}

func TestCategoryBudgetDisplayBoundaryAndStages(t *testing.T) {
	limit := ordinary.DefaultLimits().MaxBytes / 3
	for _, tc := range []struct {
		name, raw, want string
		resource        bool
	}{
		{name: "ascii-at", raw: strings.Repeat("x", limit), want: strings.Repeat("x", limit)},
		{name: "ascii-plus-one", raw: strings.Repeat("x", limit+1), resource: true},
		{name: "space-at", raw: strings.Repeat(" ", limit), want: ""},
		{name: "space-plus-one", raw: strings.Repeat(" ", limit+1), resource: true},
		{name: "utf8-at", raw: strings.Repeat("界", limit/3), want: strings.Repeat("界", limit/3)},
		{name: "utf8-plus-one", raw: strings.Repeat("界", limit/3) + "x", resource: true},
		{name: "egc-at", raw: strings.Repeat(" ", limit-len("a\n\u0301b")) + "a\n\u0301b", want: "a \u0301b"},
		{name: "egc-plus-one", raw: strings.Repeat(" ", limit+1-len("a\n\u0301b")) + "a\n\u0301b", resource: true},
		{name: "empty", raw: "", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := categoryDisplayName(tc.raw)
			if tc.resource {
				if !errors.Is(err, ordinary.ErrResourceLimit) || got != "" {
					t.Fatal("missing preprocessing rejection", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("source D changed: lengths raw=%d got=%d want=%d err=%v", len(tc.raw), len(got), len(tc.want), err)
			}
		})
	}
	invalidAt := strings.Repeat(" ", limit-1) + "\xff"
	if _, err := categoryDisplayName(invalidAt); !errors.Is(err, canonical16.ErrInvalidUTF8) {
		t.Fatal("UTF8 stage changed", err)
	}
	if _, err := categoryDisplayName(invalidAt + "x"); !errors.Is(err, ordinary.ErrResourceLimit) {
		t.Fatal("oversize must precede UTF8/D", err)
	}
}

func TestCategoryBudgetLegalDisplaySemanticsUnchanged(t *testing.T) {
	for _, raw := range []string{" 工作 / 中文 ", "a\n\u0301b", "a\u00a0\u0301b", "\r\n\t工程\u2003设计  ", "👩‍👩‍👧‍👦 🇸🇬 e\u0301", "\xff"} {
		want, werr := canonical16.DisplayName(raw)
		got, err := categoryDisplayName(raw)
		if got != want || !errors.Is(err, werr) {
			t.Fatalf("UI D changed %q: %q/%v vs %q/%v", raw, got, err, want, werr)
		}
	}
}

func TestCategoryBudgetPreflightDominatesCanonicalization(t *testing.T) {
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "runtime_category.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var preflight, submit *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			switch fn.Name.Name {
			case "categoryDisplayName":
				preflight = fn
			case "submitCategory":
				submit = fn
			}
		}
	}
	if preflight == nil || submit == nil || len(preflight.Body.List) < 2 {
		t.Fatal("missing UI preprocessing boundary")
	}
	guard, ok := preflight.Body.List[0].(*ast.IfStmt)
	if !ok {
		t.Fatal("preflight must be first")
	}
	text := func(n ast.Node) string {
		var b bytes.Buffer
		if err := format.Node(&b, fs, n); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	if text(guard.Cond) != "len(raw) > ordinary.DefaultLimits().MaxBytes/3" || len(guard.Body.List) != 1 || text(guard.Body.List[0]) != `return "", ordinary.ErrResourceLimit` {
		t.Fatal("three-byte prepaid guard changed", text(guard))
	}
	callD := token.NoPos
	ast.Inspect(preflight, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && text(call.Fun) == "canonical16.DisplayName" {
			if callD != token.NoPos {
				t.Fatal("multiple UI D passes")
			}
			callD = call.Pos()
		}
		return true
	})
	if callD <= guard.End() {
		t.Fatal("canonicalization precedes resource rejection")
	}
	uiCall, idCall := token.NoPos, token.NoPos
	ast.Inspect(submit, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch text(call.Fun) {
			case "categoryDisplayName":
				uiCall = call.Pos()
			case "core.NewIdentity":
				idCall = call.Pos()
			case "canonical16.DisplayName":
				t.Fatal("submit bypassed preflight")
			}
		}
		return true
	})
	if uiCall == token.NoPos || idCall == token.NoPos || uiCall >= idCall {
		t.Fatal("ID creation precedes UI preprocessing")
	}
}
