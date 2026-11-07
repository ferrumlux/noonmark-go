// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Version 2 explicitly adds one typed owner to the historical two-owner rule.
// This is strict matching of reviewed function ASTs, not general control-flow
// or whole-program alias analysis. Comments and source positions confer no authority.
const ordinaryOwnerContractSHA256 = "cb307a98ab35a79e0d59badcd5002bfa8b5223d8eef0311d7771548abd72d428"

type ordinaryOwnerStatement struct {
	Label  string `json:"label"`
	SHA256 string `json:"sha256"`
}
type ordinaryOwnerFunction struct {
	File       string                   `json:"file"`
	Function   string                   `json:"function"`
	Signature  string                   `json:"signature_sha256"`
	Statements []ordinaryOwnerStatement `json:"statements"`
}

func ordinaryOwnerNormalized(n any) (string, error) {
	var b bytes.Buffer
	err := format.Node(&b, token.NewFileSet(), n)
	return b.String(), err
}
func ordinaryOwnerHash(n any) (string, error) {
	text, err := ordinaryOwnerNormalized(n)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text))), err
}
func ordinaryOwnerContracts(t *testing.T) []ordinaryOwnerFunction {
	t.Helper()
	b, err := os.ReadFile("testdata/ordinary_owner_contract_v2.json")
	must(t, err)
	if fmt.Sprintf("%x", sha256.Sum256(b)) != ordinaryOwnerContractSHA256 {
		t.Fatal("versioned ownership contract changed")
	}
	var contracts []ordinaryOwnerFunction
	must(t, json.Unmarshal(b, &contracts))
	if len(contracts) != 5 {
		t.Fatal("incomplete ownership contract")
	}
	return contracts
}
func ordinaryOwnerSources(t *testing.T) map[string]*ast.File {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	must(t, err)
	result := map[string]*ast.File{}
	for _, path := range paths {
		if !strings.HasSuffix(path, "_test.go") {
			f, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			must(t, e)
			result[path] = f
		}
	}
	return result
}
func ordinaryOwnerFind(files map[string]*ast.File, path, name string) (*ast.FuncDecl, error) {
	f := files[path]
	if f == nil {
		return nil, fmt.Errorf("missing ownership source %s", path)
	}
	var found *ast.FuncDecl
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			if found != nil {
				return nil, fmt.Errorf("duplicate owner function %s:%s", path, name)
			}
			found = fn
		}
	}
	if found == nil || found.Body == nil {
		return nil, fmt.Errorf("missing owner function %s:%s", path, name)
	}
	return found, nil
}

// Inputs are explicit so independent tests can apply syntactically valid AST
// mutations to this exact checker, without compiling or editing product files.
func checkOrdinaryOwnerContract(files map[string]*ast.File, contracts []ordinaryOwnerFunction) error {
	found := map[string]int{}
	for path, f := range files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					name := outcomeSelectorName(c.Fun)
					if strings.HasPrefix(name, "s.disk.") && (strings.Contains(name, "commit") || strings.Contains(name, "verifyPrefix")) {
						found[path+":"+fn.Name.Name+":"+name]++
					}
				}
				return true
			})
		}
	}
	want := map[string]int{"outcome.go:commitLocked:s.disk.commitObserved": 1, "outcome.go:verifyPrefixLocked:s.disk.verifyPrefixObserved": 1, "ordinary_service.go:persistOrdinaryLocked:s.disk.commitOrdinaryObserved": 1}
	if !reflect.DeepEqual(found, want) {
		return fmt.Errorf("version 2 live disk ownership: %v", found)
	}
	for _, c := range contracts {
		fn, err := ordinaryOwnerFind(files, c.File, c.Function)
		if err != nil {
			return err
		}
		sig := *fn
		sig.Body = nil
		hash, err := ordinaryOwnerHash(&sig)
		if err != nil {
			return err
		}
		if hash != c.Signature {
			return fmt.Errorf("%s:%s signature/receiver/typed-input changed", c.File, c.Function)
		}
		if len(fn.Body.List) != len(c.Statements) {
			return fmt.Errorf("%s:%s statement count/order changed", c.File, c.Function)
		}
		for i, statement := range fn.Body.List {
			hash, err = ordinaryOwnerHash(statement)
			if err != nil {
				return err
			}
			if hash != c.Statements[i].SHA256 {
				return fmt.Errorf("%s:%s reviewed control-flow segment %s changed", c.File, c.Function, c.Statements[i].Label)
			}
		}
	}
	return nil
}
func TestOrdinaryOwnerContractV2(t *testing.T) {
	must(t, checkOrdinaryOwnerContract(ordinaryOwnerSources(t), ordinaryOwnerContracts(t)))
}

func TestOrdinaryOwnerContractStructuralMutants(t *testing.T) {
	contracts := ordinaryOwnerContracts(t)
	original := ordinaryOwnerSources(t)
	// Reparse only the selected source. Other syntax trees are never mutated.
	mutate := func(t *testing.T, c ordinaryOwnerFunction, edit func(*ast.FuncDecl)) {
		t.Helper()
		files := map[string]*ast.File{}
		for p, f := range original {
			files[p] = f
		}
		f, err := parser.ParseFile(token.NewFileSet(), c.File, nil, 0)
		must(t, err)
		files[c.File] = f
		fn, err := ordinaryOwnerFind(files, c.File, c.Function)
		must(t, err)
		edit(fn)
		// The mutation is fed directly to the same checker. A Go compiler failure
		// is never counted as rejection evidence.
		text, err := ordinaryOwnerNormalized(f)
		must(t, err)
		_, err = parser.ParseFile(token.NewFileSet(), c.File, text, 0)
		must(t, err)
		if err = checkOrdinaryOwnerContract(files, contracts); err == nil {
			t.Fatal("ownership mutation accepted")
		}
	}
	total := 0
	for _, c := range contracts {
		fn, err := ordinaryOwnerFind(original, c.File, c.Function)
		must(t, err)
		for i := range fn.Body.List {
			total++
			t.Run(c.Function+fmt.Sprintf("/delete_statement_%d", i), func(t *testing.T) {
				mutate(t, c, func(f *ast.FuncDecl) { f.Body.List = append(f.Body.List[:i], f.Body.List[i+1:]...) })
			})
			if i+1 < len(fn.Body.List) {
				left, _ := ordinaryOwnerNormalized(fn.Body.List[i])
				right, _ := ordinaryOwnerNormalized(fn.Body.List[i+1])
				if left != right {
					total++
					t.Run(c.Function+fmt.Sprintf("/swap_statement_%d", i), func(t *testing.T) {
						mutate(t, c, func(f *ast.FuncDecl) { f.Body.List[i], f.Body.List[i+1] = f.Body.List[i+1], f.Body.List[i] })
					})
				}
			}
		}
		// Every nested guard must retain its actual rejection and exact predicate.
		var branches []*ast.IfStmt
		var comparisons []*ast.BinaryExpr
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if x, ok := n.(*ast.IfStmt); ok {
				branches = append(branches, x)
			}
			if x, ok := n.(*ast.BinaryExpr); ok {
				switch x.Op {
				case token.EQL, token.NEQ, token.LAND, token.LOR, token.GTR, token.GEQ, token.LSS, token.LEQ:
					comparisons = append(comparisons, x)
				}
			}
			return true
		})
		for i := range branches {
			for _, mode := range []string{"true", "false", "no rejection", "unreachable"} {
				total++
				t.Run(c.Function+fmt.Sprintf("/branch_%d_%s", i, mode), func(t *testing.T) {
					mutate(t, c, func(f *ast.FuncDecl) {
						var list []*ast.IfStmt
						ast.Inspect(f.Body, func(n ast.Node) bool {
							if x, ok := n.(*ast.IfStmt); ok {
								list = append(list, x)
							}
							return true
						})
						x := list[i]
						switch mode {
						case "true", "false":
							x.Cond = ast.NewIdent(mode)
						case "no rejection":
							x.Body.List = nil
						case "unreachable":
							x.Body.List = []ast.Stmt{&ast.IfStmt{Cond: ast.NewIdent("false"), Body: &ast.BlockStmt{List: x.Body.List}}}
						}
					})
				})
			}
		}
		for i := range comparisons {
			total++
			t.Run(c.Function+fmt.Sprintf("/reverse_comparison_%d", i), func(t *testing.T) {
				mutate(t, c, func(f *ast.FuncDecl) {
					var list []*ast.BinaryExpr
					ast.Inspect(f.Body, func(n ast.Node) bool {
						if x, ok := n.(*ast.BinaryExpr); ok {
							switch x.Op {
							case token.EQL, token.NEQ, token.LAND, token.LOR, token.GTR, token.GEQ, token.LSS, token.LEQ:
								list = append(list, x)
							}
						}
						return true
					})
					x := list[i]
					x.Op = map[token.Token]token.Token{token.EQL: token.NEQ, token.NEQ: token.EQL, token.LAND: token.LOR, token.LOR: token.LAND, token.GTR: token.LEQ, token.GEQ: token.LSS, token.LSS: token.GEQ, token.LEQ: token.GTR}[x.Op]
				})
			})
		}
	}
	t.Logf("structural AST mutation cases: %d", total)
}

func TestOrdinaryOwnerContractNamedBypasses(t *testing.T) {
	contracts := ordinaryOwnerContracts(t)
	originals := ordinaryOwnerSources(t)
	original, err := os.ReadFile("ordinary_service.go")
	must(t, err)
	owner := func(text string) map[string]*ast.File {
		files := map[string]*ast.File{}
		for p, f := range originals {
			files[p] = f
		}
		f, e := parser.ParseFile(token.NewFileSet(), "ordinary_service.go", text, 0)
		must(t, e)
		files["ordinary_service.go"] = f
		return files
	}
	variants := map[string]string{
		"method value plus dead decoy": strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "write := s.disk.commitOrdinaryObserved; o := write(c); if false { s.disk.commitOrdinaryObserved(c) }", 1),
		"disk alias plus dead decoy":   strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "disk := s.disk; o := disk.commitOrdinaryObserved(c); if false { s.disk.commitOrdinaryObserved(c) }", 1),
		"duplicate writer":             strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "o := s.disk.commitOrdinaryObserved(c); o = s.disk.commitOrdinaryObserved(c)", 1),
		"raw in owner":                 strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "s.disk.commitObserved(s.snapshot); o := s.disk.commitOrdinaryObserved(c)", 1),
		"raw other function":           string(original) + "\nfunc (s *Service) extraOwner() { s.disk.commitObserved(s.snapshot) }\n",
		"rename owner":                 strings.ReplaceAll(string(original), "persistOrdinaryLocked", "renamedOrdinaryOwner"),
		"legacy callee":                strings.Replace(string(original), "s.disk.commitOrdinaryObserved(c)", "s.disk.commitObserved(s.snapshot)", 1),
		"method value":                 strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "write := s.disk.commitOrdinaryObserved; o := write(c)", 1),
		"disk alias":                   strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "disk := s.disk; o := disk.commitOrdinaryObserved(c)", 1),
		"receiver alias":               strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "svc := s; o := svc.disk.commitOrdinaryObserved(c)", 1),
		"discard observation":          strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "s.disk.commitOrdinaryObserved(c); o := commitObservation{}", 1),
		"wrong attempt":                strings.Replace(string(original), "setAttempt(a, o.extent, o.stage)", "setAttempt(a, WriteNone, StageNone)", 1),
		"wrong invalidation":           strings.Replace(string(original), "s.invalidateLocked(o.reason, o.stage)", "s.invalidateLocked(HealthOK, StageNone)", 1),
		"failure publish":              strings.Replace(string(original), "if o.err != nil {\n\t\treturn o.err", "if o.err != nil {\n\t\ts.aggregate = &c.after; s.graph = g; s.snapshot = wire; return o.err", 1),
		"early publish":                strings.Replace(string(original), "o := s.disk.commitOrdinaryObserved(c)", "s.aggregate = &c.after; s.graph = g; s.snapshot = wire; o := s.disk.commitOrdinaryObserved(c)", 1),
		"raw aggregate":                strings.Replace(string(original), "s.aggregate = s.disk.ordinaryHead", "s.aggregate = &c.after", 1),
	}
	for name, text := range variants {
		t.Run(name, func(t *testing.T) {
			if text == string(original) {
				t.Fatal("mutation did not apply")
			}
			if checkOrdinaryOwnerContract(owner(text), contracts) == nil {
				t.Fatal("bypass accepted")
			}
		})
	}
	for _, name := range []string{"wrong file", "missing legacy commit", "missing legacy prefix"} {
		t.Run(name, func(t *testing.T) {
			files := owner(string(original))
			if name == "wrong file" {
				files["moved.go"] = files["ordinary_service.go"]
				delete(files, "ordinary_service.go")
			} else {
				text, e := os.ReadFile("outcome.go")
				must(t, e)
				old := "s.disk.commitObserved"
				if name == "missing legacy prefix" {
					old = "s.disk.verifyPrefixObserved"
				}
				text = []byte(strings.Replace(string(text), old, "s.disk.disallowed", 1))
				f, e := parser.ParseFile(token.NewFileSet(), "outcome.go", text, 0)
				must(t, e)
				files["outcome.go"] = f
			}
			if checkOrdinaryOwnerContract(files, contracts) == nil {
				t.Fatal("ownership set mutation accepted")
			}
		})
	}
}
