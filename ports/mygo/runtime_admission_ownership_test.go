// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

// These bind whole files, including imports, helper methods and comments. The
// old function-level ownership hash remains independently enforced downstream.
// Only the reviewed, frozen v4 source may reconstruct the exact prior main.
const runtimeAdmissionMainBeforeSHA256 = "a69e32cc56991f14a3e082f5e6cc6ee000cd7eeca2bdb9ee414a4cba0f693077"
const runtimeAdmissionMainAfterSHA256 = "426fdf2864efefebbd0c54f9c09da91364a3b16b728ba2915524075a4da0cf48"

func runtimeAdmissionMainContract(before, after string) (string, error) {
	if fmt.Sprintf("%x", sha256.Sum256([]byte(before))) != runtimeAdmissionMainBeforeSHA256 || fmt.Sprintf("%x", sha256.Sum256([]byte(after))) != runtimeAdmissionMainAfterSHA256 {
		return "", errors.New("unreviewed admission main source bytes")
	}
	if err := runtimeAdmissionMainLifecycleAST(after); err != nil {
		return "", err
	}
	// The predecessor must still pass its unchanged complete-main and four-owner
	// proofs; reconstruction is not permission to bypass those historical guards.
	if _, err := runtimeExitLegacyOwnershipSource(before); err != nil {
		return "", err
	}
	return before, nil
}
func runtimeAdmissionLegacyMainSource(source string) (string, error) {
	before, err := os.ReadFile("docs/tasks/runtime-admission-lifecycle/MAIN-BEFORE-v4.go.txt")
	if err != nil {
		return "", err
	}
	return runtimeAdmissionMainContract(string(before), source)
}
func TestRuntimeAdmissionMainRejectsEveryByteMutation(t *testing.T) {
	before, err := os.ReadFile("docs/tasks/runtime-admission-lifecycle/MAIN-BEFORE-v4.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	got, err := runtimeAdmissionMainContract(string(before), string(after))
	if err != nil || got != string(before) {
		t.Fatalf("main inverse is not exact: %v", err)
	}
	originals := []string{string(before), string(after)}
	for part, raw := range originals {
		for i := range []byte(raw) {
			args := append([]string(nil), originals...)
			bad := []byte(raw)
			bad[i] ^= 1
			args[part] = string(bad)
			if result, err := runtimeAdmissionMainContract(args[0], args[1]); err == nil || result != "" {
				t.Fatalf("accepted main %d byte %d", part, i)
			}
		}
	}
	for _, changed := range []string{"", string(after) + "\n", strings.ReplaceAll(string(after), "\n", "\r\n")} {
		if _, err := runtimeAdmissionMainContract(string(before), changed); err == nil {
			t.Fatal("accepted malformed main")
		}
	}
}

func TestRuntimeAdmissionMainOwnershipConsumerChain(t *testing.T) {
	for _, consumer := range []struct{ file, name string }{
		{"runtime_exit_cleanup_test.go", "TestRuntimeExitMainHasSingleClosePerPath"},
		{"runtime_exit_business_ownership_test.go", "TestRuntimeExitBusinessOwnershipRejectsPathMutations"},
		{"runtime_exit_business_ownership_test.go", "TestRuntimeExitBusinessWholeMainSourceContract"},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), consumer.file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		var body *ast.BlockStmt
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == consumer.name {
				body = fn.Body
			}
		}
		if body == nil {
			t.Fatal("missing main guard consumer", consumer.name)
		}
		var calls []string
		priorName, rawName := "source", "raw"
		if consumer.name == "TestRuntimeExitMainHasSingleClosePerPath" {
			priorName, rawName = "prior", "b"
		}
		checkInput := func(call *ast.CallExpr, output ast.Expr, name string) {
			t.Helper()
			want := priorName
			if name == "runtimeAdmissionLegacyMainSource" {
				want = "string(" + rawName + ")"
				if id, ok := output.(*ast.Ident); !ok || id.Name != priorName {
					t.Fatal("reconstructed main not assigned to historical input")
				}
			}
			if len(call.Args) != 1 || runtimeExitNodeText(call.Args[0]) != want {
				t.Fatal("main source substituted between guards")
			}
		}
		for _, stmt := range body.List {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 {
				continue
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				continue
			}
			if fn.Name == "runtimeAdmissionLegacyMainSource" || fn.Name == "runtimeExitLegacyOwnershipSource" {
				if len(call.Args) != 1 || len(assign.Lhs) != 2 {
					t.Fatal("main reconstruction result dropped")
				}
				checkInput(call, assign.Lhs[0], fn.Name)
				calls = append(calls, fn.Name)
			}
		}
		// The whole-main consumer invokes its historical guard in an if initializer.
		if consumer.name == "TestRuntimeExitBusinessWholeMainSourceContract" {
			for _, stmt := range body.List {
				if v, ok := stmt.(*ast.IfStmt); ok && v.Init != nil {
					if a, ok := v.Init.(*ast.AssignStmt); ok && len(a.Rhs) == 1 {
						if c, ok := a.Rhs[0].(*ast.CallExpr); ok {
							if f, ok := c.Fun.(*ast.Ident); ok && f.Name == "runtimeExitLegacyOwnershipSource" {
								checkInput(c, a.Lhs[0], f.Name)
								calls = append(calls, f.Name)
							}
						}
					}
				}
			}
		}
		if !reflect.DeepEqual(calls, []string{"runtimeAdmissionLegacyMainSource", "runtimeExitLegacyOwnershipSource"}) {
			t.Fatal("main guards must run v4 before historical ownership", consumer.name, calls)
		}
	}
}

const runtimeAdmissionEnableFailureBlock = `if err := hub.enableCategoryCandidate(); err != nil {
 admission.EndRun()
 admission.WaitActiveUI()
 <-writer.Done()
 admission.FinishWriterDrain()
 <-dispatcher.Done()
 admission.FinishDispatchDrain()
 closeErr := service.Close()
 log.Fatal(startup.startupFailure("窗口初始化", errors.Join(err, closeErr)))
}`
const runtimeAdmissionWriterDrainBlock = `if writer != nil {
 admission.WaitActiveUI()
 <-writer.Done()
 if probe != nil { probe.RecordDrain(true) }
 if business != nil { businessCleanupErr = business.PreClose() }
 admission.FinishWriterDrain()
 <-dispatcher.Done()
 lifecycle := admission.FinishDispatchDrain()
 if lifecycle.ProtocolErrors != 0 || lifecycle.CompletionRejected != 0 {
  businessCleanupErr = errors.Join(businessCleanupErr, fmt.Errorf("runtime admission protocol errors: %d; rejected completions: %d", lifecycle.ProtocolErrors, lifecycle.CompletionRejected))
 }
 log.Printf("runtime admission: %+v", lifecycle)
 if probe != nil { probe.RecordDrain(false) }
}`

func runtimeAdmissionMainLifecycleAST(source string) error {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", source, 0)
	if err != nil {
		return err
	}
	var main *ast.FuncDecl
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "main" {
			if main != nil {
				return errors.New("duplicate main")
			}
			main = f
		}
	}
	if main == nil || main.Body == nil || runtimeExitCloseCalls(main) != 4 {
		return errors.New("main owner count changed")
	}
	native, ni := runtimeExitFindIf(main.Body, "service != nil")
	if native == nil {
		return errors.New("missing service owner")
	}
	ordinary, _ := runtimeExitFindIf(native.Body, "startup.profile == ordinaryDevelopment")
	if ordinary == nil || ordinary.Else != nil || len(ordinary.Body.List) != 1 || !runtimeExitSameStatement(ordinary.Body.List[0], runtimeAdmissionEnableFailureBlock) {
		return errors.New("startup drain owner changed")
	}
	runIndex, cleanupIndex := -1, -1
	var cleanup *ast.FuncLit
	for i, stmt := range main.Body.List {
		if runtimeExitSameStatement(stmt, "runErr := mygo.App.Run()") {
			runIndex = i
		}
		a, ok := stmt.(*ast.AssignStmt)
		if !ok || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
			continue
		}
		lhs, ok := a.Lhs[0].(*ast.Ident)
		if !ok || lhs.Name != "code" {
			continue
		}
		call, ok := a.Rhs[0].(*ast.CallExpr)
		if !ok {
			continue
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok || fn.Name != "runtimeExitStatus" || len(call.Args) != 2 || runtimeExitNodeText(call.Args[0]) != "runErr" {
			continue
		}
		if cleanup != nil {
			return errors.New("duplicate cleanup")
		}
		cleanup, ok = call.Args[1].(*ast.FuncLit)
		if !ok {
			return errors.New("nonliteral cleanup")
		}
		cleanupIndex = i
	}
	if runIndex < 0 || runIndex+1 >= len(main.Body.List) || !runtimeExitSameStatement(main.Body.List[runIndex+1], "if admission != nil { admission.EndRun() }") {
		return errors.New("EndRun not immediate")
	}
	if cleanup == nil || !(ni < runIndex && runIndex < cleanupIndex) || len(cleanup.Body.List) != 3 || !runtimeExitSameStatement(cleanup.Body.List[0], runtimeAdmissionWriterDrainBlock) || !runtimeExitSameStatement(cleanup.Body.List[1], runtimeExitNativeCloseBlock) || !runtimeExitSameStatement(cleanup.Body.List[2], "return businessCleanupErr") {
		return errors.New("cleanup drain/close ordering changed")
	}
	return nil
}

func TestRuntimeAdmissionMainLifecycleASTRejectsMutations(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if err := runtimeAdmissionMainLifecycleAST(source); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{
		{"admission.EndRun()", "admission.FinishWriterDrain()"},
		{"admission.WaitActiveUI()", "admission.EndRun()"},
		{"<-writer.Done()", "<-dispatcher.Done()"},
		{"admission.FinishWriterDrain()", "dispatcher.StopAfterDrain()"},
		{"<-dispatcher.Done()", "<-writer.Done()"},
		{"admission.FinishDispatchDrain()", "admission.FinishWriterDrain()"},
		{"businessCleanupErr = business.PreClose()", "_ = business.PreClose()"},
		{"return errors.Join(err, businessCleanupErr)", "return err"},
	} {
		if !strings.Contains(source, pair[0]) {
			t.Fatal("missing lifecycle mutation anchor", pair[0])
		}
		if err := runtimeAdmissionMainLifecycleAST(strings.ReplaceAll(source, pair[0], pair[1])); err == nil {
			t.Fatal("accepted lifecycle mutation", pair[0])
		}
	}
}
