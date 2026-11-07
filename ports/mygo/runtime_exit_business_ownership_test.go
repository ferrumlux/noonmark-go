// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This is one narrowly reviewed early owner, not permission for an arbitrary
// fourth Close. The old three-owner test is run on reconstructed source only
// after this branch and all four mutually exclusive paths have been verified.
// Exact reviewed d691 main declaration, not a Go dataflow/equivalence proof.
// Bind the entire finite function so aliases, closures, shadows and helper
// calls cannot be inserted at uninspected positions. Never derive expected
// from the caller's source; changing it requires renewed ownership review.
const runtimeExitMainOwnershipSHA256 = "1a309f07e4e3164980dd3818e125aae72d71ff8c64b54d4d900c946803ebe7ee"

const runtimeExitBusinessBoundaryBlock = `if business != nil {
 business.service = service
 if err := business.sample("after_open"); err != nil {
  business.fail(err)
  closeErr := service.Close()
  log.Fatal(startup.startupFailure("日期边界", errors.Join(err, closeErr)))
 }
}`
const runtimeExitFactoryFailureBlock = `if err != nil {
 log.Fatal(startup.startupFailure("会话启动", err))
}`
const runtimeExitEnableFailureBlock = `if err := hub.enableCategoryCandidate(); err != nil {
 writer.StopAfterDrain()
 <-writer.Done()
 dispatcher.StopAfterDrain()
 <-dispatcher.Done()
 closeErr := service.Close()
 log.Fatal(startup.startupFailure("窗口初始化", errors.Join(err, closeErr)))
}`
const runtimeExitNativeCloseBlock = `if service != nil {
 err := service.Close()
 if probe != nil { probe.RecordServiceClose(err) }
 return errors.Join(err, businessCleanupErr)
}`

func runtimeExitNodeText(n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, token.NewFileSet(), n); err != nil {
		panic(err)
	}
	return b.String()
}
func runtimeExitStatement(text string) ast.Stmt {
	f, err := parser.ParseFile(token.NewFileSet(), "contract.go", "package main\nfunc contract(){\n"+text+"\n}", 0)
	if err != nil {
		panic(err)
	}
	body := f.Decls[0].(*ast.FuncDecl).Body.List
	if len(body) != 1 {
		panic("contract must be exactly one statement")
	}
	return body[0]
}
func runtimeExitSameStatement(n ast.Stmt, text string) bool {
	return runtimeExitNodeText(n) == runtimeExitNodeText(runtimeExitStatement(text))
}
func runtimeExitCondition(n ast.Expr, text string) bool {
	want, err := parser.ParseExpr(text)
	if err != nil {
		panic(err)
	}
	return runtimeExitNodeText(n) == runtimeExitNodeText(want)
}
func runtimeExitCloseCalls(n ast.Node) int {
	count := 0
	ast.Inspect(n, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		s, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := s.X.(*ast.Ident)
		if ok && id.Name == "service" && s.Sel.Name == "Close" {
			count++
		}
		return true
	})
	return count
}
func runtimeExitFindIf(body *ast.BlockStmt, condition string) (*ast.IfStmt, int) {
	var found *ast.IfStmt
	index := -1
	for i, n := range body.List {
		v, ok := n.(*ast.IfStmt)
		if ok && v.Init == nil && runtimeExitCondition(v.Cond, condition) {
			if found != nil {
				return nil, -1
			}
			found, index = v, i
		}
	}
	return found, index
}

func runtimeExitLegacyOwnershipSource(source string) (string, error) {
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "main.go", source, 0)
	if err != nil {
		return "", err
	}
	// Bind the terminal/error functions to their standard-library imports.
	for _, name := range []string{"log", "errors"} {
		found := false
		for _, imp := range file.Imports {
			if imp.Path.Value == `"`+name+`"` && imp.Name == nil {
				found = true
			}
		}
		if !found {
			return "", fmt.Errorf("unbound standard import %s", name)
		}
	}
	var main *ast.FuncDecl
	for _, n := range file.Decls {
		if f, ok := n.(*ast.FuncDecl); ok && f.Name.Name == "main" {
			if main != nil {
				return "", errors.New("duplicate main")
			}
			main = f
		}
	}
	if main == nil || main.Body == nil || runtimeExitCloseCalls(main) != 4 {
		return "", errors.New("unexpected actual Service ownership calls")
	}
	startMain, endMain := fs.Position(main.Pos()).Offset, fs.Position(main.End()).Offset
	if fmt.Sprintf("%x", sha256.Sum256([]byte(source[startMain:endMain]))) != runtimeExitMainOwnershipSHA256 {
		return "", errors.New("unreviewed complete main ownership source")
	}
	// Local aliases/shadows cannot turn the inspected Fatal into a returning
	// function, replace Join, or hide a Close method value outside the count.
	unsafeBinding := false
	closeSelectors, directCloses := 0, map[*ast.SelectorExpr]bool{}
	ast.Inspect(main, func(n ast.Node) bool {
		bad := func(name string) {
			if name == "log" || name == "errors" {
				unsafeBinding = true
			}
		}
		switch v := n.(type) {
		case *ast.AssignStmt:
			for _, x := range v.Lhs {
				if id, ok := x.(*ast.Ident); ok {
					bad(id.Name)
				}
			}
		case *ast.ValueSpec:
			for _, id := range v.Names {
				bad(id.Name)
			}
		case *ast.Field:
			for _, id := range v.Names {
				bad(id.Name)
			}
		case *ast.RangeStmt:
			for _, x := range []ast.Expr{v.Key, v.Value} {
				if id, ok := x.(*ast.Ident); ok {
					bad(id.Name)
				}
			}
		case *ast.CallExpr:
			if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "service" && sel.Sel.Name == "Close" && len(v.Args) == 0 {
					directCloses[sel] = true
				}
			}
		case *ast.SelectorExpr:
			if id, ok := v.X.(*ast.Ident); ok && id.Name == "service" && v.Sel.Name == "Close" {
				closeSelectors++
			}
		}
		return true
	})
	if unsafeBinding || closeSelectors != 4 || len(directCloses) != closeSelectors {
		return "", errors.New("shadowed standard function or aliased Close")
	}
	session, si := runtimeExitFindIf(main.Body, `*initSession != "" || *session != ""`)
	render, ri := runtimeExitFindIf(main.Body, `*render != "" || *one != ""`)
	native, ni := runtimeExitFindIf(main.Body, `service != nil`)
	if session == nil || render == nil || native == nil || session.Else != nil || render.Else != nil || native.Else != nil || !(si < ri && ri < ni) {
		return "", errors.New("owner path topology")
	}
	var boundary ast.Stmt
	factoryIndex := -1
	for i, n := range session.Body.List {
		if runtimeExitSameStatement(n, `service, err = startup.openService(today)`) {
			if factoryIndex != -1 {
				return "", errors.New("duplicate service factory")
			}
			factoryIndex = i
		}
	}
	if factoryIndex < 0 || factoryIndex+2 >= len(session.Body.List) || !runtimeExitSameStatement(session.Body.List[factoryIndex+1], runtimeExitFactoryFailureBlock) {
		return "", errors.New("factory success gate missing")
	}
	// Keep the factory at the reviewed point in the session block. An extra
	// lane/hub acquisition cannot precede this early cleanup unnoticed.
	if factoryIndex != 6 || len(session.Body.List) != 10 {
		return "", errors.New("unreviewed pre-owner session work")
	}
	boundary = session.Body.List[factoryIndex+2]
	if !runtimeExitSameStatement(boundary, runtimeExitBusinessBoundaryBlock) || runtimeExitCloseCalls(session) != 1 {
		return "", errors.New("date-boundary owner or terminal propagation changed")
	}
	earlyOwner := false
	ast.Inspect(main, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || c.Pos() >= boundary.End() {
			return true
		}
		name := runtimeExitNodeText(c.Fun)
		switch name {
		case "coordinator.NewSerialLane", "newTitleHub", "newTitleDispatch", "newRuntimeApp", "mygo.NewWindow", "mygo.App.Run":
			earlyOwner = true
		}
		return true
	})
	if earlyOwner {
		return "", errors.New("lane/hub/window owns service before date boundary")
	}
	// A date failure terminates through log.Fatal before render/lane ownership.
	// Render returns before the native path, even when no Service was opened.
	renderClose, _ := runtimeExitFindIf(render.Body, `service != nil`)
	renderFailure, _ := runtimeExitFindIf(render.Body, `renderErr != nil`)
	if renderClose == nil || !runtimeExitSameStatement(renderClose, `if service != nil { renderErr = errors.Join(renderErr, service.Close()) }`) || renderFailure == nil || !runtimeExitSameStatement(renderFailure, `if renderErr != nil { log.Fatal(renderErr) }`) || !runtimeExitSameStatement(render.Body.List[len(render.Body.List)-1], `return`) || runtimeExitCloseCalls(render) != 1 {
		return "", errors.New("render owner changed")
	}
	ordinary, _ := runtimeExitFindIf(native.Body, `startup.profile == ordinaryDevelopment`)
	if ordinary == nil || ordinary.Else != nil || len(ordinary.Body.List) != 1 || !runtimeExitSameStatement(ordinary.Body.List[0], runtimeExitEnableFailureBlock) || runtimeExitCloseCalls(native) != 1 {
		return "", errors.New("failed-enable owner changed")
	}
	cleanupIndex := -1
	var cleanup *ast.FuncLit
	runIndex := -1
	for i, n := range main.Body.List {
		if runtimeExitSameStatement(n, `runErr := mygo.App.Run()`) {
			runIndex = i
		}
		a, ok := n.(*ast.AssignStmt)
		if !ok || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
			continue
		}
		id, ok := a.Lhs[0].(*ast.Ident)
		if !ok || id.Name != "code" {
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
			return "", errors.New("duplicate native cleanup")
		}
		cleanup, ok = call.Args[1].(*ast.FuncLit)
		if !ok {
			return "", errors.New("native cleanup not direct callback")
		}
		cleanupIndex = i
	}
	if cleanup == nil || !(ni < runIndex && runIndex < cleanupIndex) || len(cleanup.Body.List) < 2 || runtimeExitCloseCalls(cleanup) != 1 {
		return "", errors.New("native owner not after event loop")
	}
	normal, _ := runtimeExitFindIf(cleanup.Body, `service != nil`)
	if normal == nil || !runtimeExitSameStatement(normal, runtimeExitNativeCloseBlock) || !runtimeExitSameStatement(cleanup.Body.List[len(cleanup.Body.List)-1], `return businessCleanupErr`) {
		return "", errors.New("native Close error lost")
	}
	// Removing only the fully validated new branch restores the historical three
	// ownership paths. All existing textual propagation/order assertions remain.
	start, end := fs.Position(boundary.Pos()).Offset, fs.Position(boundary.End()).Offset
	return source[:start] + source[end:], nil
}

func TestRuntimeExitBusinessOwnershipRejectsPathMutations(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	legacy, err := runtimeExitLegacyOwnershipSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(legacy, "service.Close()") != 3 || strings.Contains(legacy, `business.sample("after_open")`) {
		t.Fatal("historical reconstruction not exact")
	}
	cases := []struct{ name, old, new string }{
		{"duplicate-boundary-close", "business.fail(err)\n\t\t\t\tcloseErr := service.Close()", "business.fail(err)\n\t\t\t\tservice.Close()\n\t\t\t\tcloseErr := service.Close()"},
		{"remove-boundary-close", "business.fail(err)\n\t\t\t\tcloseErr := service.Close()", "business.fail(err)\n\t\t\t\tcloseErr := error(nil)"},
		{"boundary-defer", "business.fail(err)\n\t\t\t\tcloseErr := service.Close()", "business.fail(err)\n\t\t\t\tdefer service.Close()\n\t\t\t\tcloseErr := error(nil)"},
		{"unguarded-business", "if business != nil {\n\t\t\tbusiness.service = service", "if true {\n\t\t\tbusiness.service = service"},
		{"wrong-date-failure-predicate", `if err := business.sample("after_open"); err != nil`, `if err := business.sample("after_open"); err == nil`},
		{"different-factory", `service, err = startup.openService(today)`, `service, err = startup.otherService(today)`},
		{"nonstandard-log-import", `"log"`, `log "example.invalid/log"`},
		{"nonstandard-errors-import", `"errors"`, `errors "example.invalid/errors"`},
		{"shadow-log-before-session", "var service *taskapp.Service", "var service *taskapp.Service\nlog := fakeLogger"},
		{"shadow-errors-before-session", "var service *taskapp.Service", "var service *taskapp.Service\nerrors := fakeErrors"},
		{"alias-extra-close", "business.fail(err)\n\t\t\t\tcloseErr := service.Close()", "business.fail(err)\n\t\t\t\tcloseAgain := service.Close\n\t\t\t\tcloseAgain()\n\t\t\t\tcloseErr := service.Close()"},
		{"early-lane-before-session", "var service *taskapp.Service", "var service *taskapp.Service\nwriter := coordinator.NewSerialLane(); _ = writer"},
		{"early-lane-inside-session", `service, err = startup.openService(today)`, `writer := coordinator.NewSerialLane(); _ = writer; service, err = startup.openService(today)`},
		{"early-hub-inside-session", `service, err = startup.openService(today)`, `hub := newTitleHub(service, writer, post, clock, now, quit); _ = hub; service, err = startup.openService(today)`},
		{"alias-after-render-before-native", "\tvar hub *titleHub", "\tvar hub *titleHub\n\talias := service; if alias != nil { alias.Close() }"},
		{"alias-immediately-after-session", "\tif service == nil && !*prototype", "\talias := service; if alias != nil { alias.Close() }\n\tif service == nil && !*prototype"},
		{"alias-before-native-cleanup", "\tvar businessCleanupErr error", "\talias := service; if alias != nil { alias.Close() }\n\tvar businessCleanupErr error"},
		{"captured-service-before-native-cleanup", "\tvar businessCleanupErr error", "\tfunc(s *taskapp.Service) { if s != nil { s.Close() } }(service)\n\tvar businessCleanupErr error"},
		{"interface-alias-before-native-cleanup", "\tvar businessCleanupErr error", "\tvar owner interface{ Close() error } = service; owner.Close()\n\tvar businessCleanupErr error"},
		{"shadow-service-before-native-cleanup", "\tvar businessCleanupErr error", "\t{ service := service; if service != nil { service.Close() } }\n\tvar businessCleanupErr error"},
		{"wrong-phase", `business.sample("after_open")`, `business.sample("startup")`},
		{"wrong-business-binding", "business.service = service", "business.service = nil"},
		{"lost-close-error", `startup.startupFailure("日期边界", errors.Join(err, closeErr))`, `startup.startupFailure("日期边界", err)`},
		{"lost-date-error", `startup.startupFailure("日期边界", errors.Join(err, closeErr))`, `startup.startupFailure("日期边界", closeErr)`},
		{"nonterminal-date-log", `log.Fatal(startup.startupFailure("日期边界", errors.Join(err, closeErr)))`, `log.Print(startup.startupFailure("日期边界", errors.Join(err, closeErr)))`},
		{"date-return-not-fatal", `log.Fatal(startup.startupFailure("日期边界", errors.Join(err, closeErr)))`, `return`},
		{"factory-error-falls-through", `log.Fatal(startup.startupFailure("会话启动", err))`, `log.Print(startup.startupFailure("会话启动", err))`},
		{"render-loses-close", `renderErr = errors.Join(renderErr, service.Close())`, `renderErr = service.Close()`},
		{"render-falls-through", "\t\treturn\n\t}\n\tvar hub *titleHub", "\t}\n\tvar hub *titleHub"},
		{"enable-nonterminal", `log.Fatal(startup.startupFailure("窗口初始化", errors.Join(err, closeErr)))`, `log.Print(startup.startupFailure("窗口初始化", errors.Join(err, closeErr)))`},
		{"native-loses-close-observation", `probe.RecordServiceClose(err)`, `probe.RecordServiceClose(nil)`},
		{"native-loses-close", `return errors.Join(err, businessCleanupErr)`, `return businessCleanupErr`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if strings.Count(source, c.old) != 1 {
				t.Fatalf("mutation anchor count: %s", c.name)
			}
			changed := strings.Replace(source, c.old, c.new, 1)
			if _, err := runtimeExitLegacyOwnershipSource(changed); err == nil {
				t.Fatal("accepted invalid ownership path")
			}
		})
	}
}

// Synthetic fault model: the exact production early-owner AST is bound below.
// This tests log.Fatal exit/error propagation, not native Close or durability.
type runtimeExitBoundaryService struct{ closeFailed bool }

func (s *runtimeExitBoundaryService) Close() error {
	fmt.Fprintln(os.Stderr, "CLOSE-OBSERVED")
	if s.closeFailed {
		return errors.New("close-sentinel")
	}
	return nil
}

type runtimeExitBoundaryBusiness struct {
	service      *runtimeExitBoundaryService
	sampleFailed bool
}

func (b *runtimeExitBoundaryBusiness) sample(phase string) error {
	if phase != "after_open" {
		panic("wrong phase")
	}
	fmt.Fprintln(os.Stderr, "SAMPLE-OBSERVED")
	if b.sampleFailed {
		return errors.New("date-sentinel")
	}
	return nil
}
func (b *runtimeExitBoundaryBusiness) fail(err error) error {
	fmt.Fprintln(os.Stderr, "FAIL-RECORDED")
	return err
}
func runtimeExitBoundaryFaultModel(business *runtimeExitBoundaryBusiness, service *runtimeExitBoundaryService, startup runtimeStartup) {
	if business != nil {
		business.service = service
		if err := business.sample("after_open"); err != nil {
			business.fail(err)
			closeErr := service.Close()
			log.Fatal(startup.startupFailure("日期边界", errors.Join(err, closeErr)))
		}
	}
	fmt.Fprintln(os.Stderr, "LATER-OWNER")
	if err := service.Close(); err != nil {
		log.Fatal(err)
	}
}
func TestRuntimeExitBusinessFaultModelMatchesProduction(t *testing.T) {
	raw, err := os.ReadFile("runtime_exit_business_ownership_test.go")
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range f.Decls {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Name.Name == "runtimeExitBoundaryFaultModel" {
			if len(fn.Body.List) < 1 || !runtimeExitSameStatement(fn.Body.List[0], runtimeExitBusinessBoundaryBlock) {
				t.Fatal("fault model differs from actual validated branch")
			}
			return
		}
	}
	t.Fatal("missing fault model")
}
func TestRuntimeExitBusinessBoundaryFatalSubprocess(t *testing.T) {
	const env = "NOONMARK_TEST_BOUNDARY_OWNER"
	if mode := os.Getenv(env); mode != "" {
		service := &runtimeExitBoundaryService{closeFailed: mode == "date-and-close-fail"}
		business := &runtimeExitBoundaryBusiness{sampleFailed: strings.HasPrefix(mode, "date-")}
		if mode == "nil-business" {
			business = nil
		}
		runtimeExitBoundaryFaultModel(business, service, runtimeStartup{profile: ordinaryDevelopment})
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"nil-business", "sample-ok", "date-fail", "date-and-close-fail"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestRuntimeExitBusinessBoundaryFatalSubprocess$")
			for _, e := range os.Environ() {
				if !strings.HasPrefix(e, env+"=") {
					cmd.Env = append(cmd.Env, e)
				}
			}
			cmd.Env = append(cmd.Env, env+"="+mode)
			raw, err := cmd.CombinedOutput()
			text := string(raw)
			if ctx.Err() != nil {
				t.Fatalf("fault model did not terminate: %v", ctx.Err())
			}
			if strings.Count(text, "CLOSE-OBSERVED") != 1 {
				t.Fatalf("close count: %s", text)
			}
			failed := strings.HasPrefix(mode, "date-")
			if failed {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 || strings.Contains(text, "LATER-OWNER") || !strings.Contains(text, "FAIL-RECORDED") || !strings.Contains(text, "date-sentinel") {
					t.Fatalf("early owner not terminal: %v %s", err, text)
				}
				if strings.Contains(text, "close-sentinel") != (mode == "date-and-close-fail") {
					t.Fatalf("close error propagation: %s", text)
				}
			} else if err != nil || !strings.Contains(text, "LATER-OWNER") || strings.Contains(text, "FAIL-RECORDED") {
				t.Fatalf("normal owner blocked: %v %s", err, text)
			}
			if strings.Contains(text, "SAMPLE-OBSERVED") != (mode != "nil-business") {
				t.Fatalf("business guard: %s", text)
			}
		})
	}
}

func TestRuntimeExitBusinessStartupFailurePreservesCauses(t *testing.T) {
	dateErr, closeErr := errors.New("date-sentinel"), errors.New("close-sentinel")
	for _, cause := range []error{dateErr, errors.Join(dateErr, closeErr)} {
		got := (runtimeStartup{profile: ordinaryDevelopment}).startupFailure("日期边界", cause)
		if !errors.Is(got, dateErr) || errors.Is(got, closeErr) != errors.Is(cause, closeErr) || !strings.Contains(got.Error(), "日期边界") {
			t.Fatal("lost startup/Close cause", got)
		}
	}
}

// The finite source contract rejects any body insertion, including one that
// does not spell service.Close at all. It deliberately also rejects formatting
// edits until the exact new source and ownership paths have been reviewed.
func TestRuntimeExitBusinessWholeMainSourceContract(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if _, err = runtimeExitLegacyOwnershipSource(source); err != nil {
		t.Fatal(err)
	}
	for _, addition := range []string{
		"\n\talias := service; if alias != nil { alias.Close() }",
		"\n\tgo func(s *taskapp.Service) { if s != nil { s.Close() } }(service)",
		"\n\tdefer func(s *taskapp.Service) { if s != nil { s.Close() } }(service)",
		"\n\tforeignOwner(service)",
		"\n\t// unreviewed body edit",
	} {
		marker := "\n\tif service == nil && !*prototype"
		if strings.Count(source, marker) != 1 {
			t.Fatal("main contract mutation anchor")
		}
		changed := strings.Replace(source, marker, addition+marker, 1)
		if _, err = runtimeExitLegacyOwnershipSource(changed); err == nil {
			t.Fatal("accepted unreviewed main insertion")
		}
	}
}
