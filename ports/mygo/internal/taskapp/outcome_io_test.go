// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type outcomeWriteFailure struct {
	storeWriteFile
	op       string
	injected error
}

func (f outcomeWriteFailure) Write(b []byte) (int, error) {
	if f.op == "short" {
		return len(b) - 1, nil
	}
	if f.op == "write" {
		return 0, f.injected
	}
	return f.storeWriteFile.Write(b)
}
func (f outcomeWriteFailure) Sync() error {
	if f.op == "sync" {
		return f.injected
	}
	return f.storeWriteFile.Sync()
}
func (f outcomeWriteFailure) Close() error {
	err := f.storeWriteFile.Close()
	if f.op == "close" {
		return f.injected
	}
	return err
}
func outcomeDiskBytes(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	must(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if d.IsDir() {
			out[rel] = "directory"
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		out[rel] = string(b)
		return nil
	}))
	return out
}
func TestOutcomeExactIOFailures(t *testing.T) {
	for _, file := range []string{"state.json", "COMMITTED"} {
		for _, op := range []string{"open", "short", "write", "sync", "close", "readback"} {
			t.Run(file+"/"+op, func(t *testing.T) {
				s, path, chain, trace := manualServiceFixture(t)
				before, rev := s.Snapshot(), s.disk.revision
				injected := errors.New("identical io failure")
				target := fmt.Sprintf("rev-%010d/%s", rev+1, file)
				s.disk.openWrite = func(name string) (storeWriteFile, error) {
					if name == target && op == "open" {
						return nil, injected
					}
					f, e := s.disk.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
					if e != nil {
						return nil, e
					}
					if name == target {
						return outcomeWriteFailure{f, op, injected}, nil
					}
					return f, nil
				}
				s.disk.readBack = func(name string, limit int) ([]byte, error) {
					if name == target && op == "readback" {
						return nil, injected
					}
					return s.disk.readRegular(name, limit)
				}
				o := s.SetManualProgressOutcome(chain, trace, 29)
				checkOutcome(t, o.Outcome, o.Receipt != nil)
				stage := StagePayload
				if file == "COMMITTED" {
					stage = StageMarker
				}
				if o.Result != ResultUncertain || o.WriteExtent != WriteReservedOrPartial || o.FailureStage != stage || !errors.Is(o.Err, ErrIncomplete) {
					t.Fatalf("IO classification: %+v", o)
				}
				if !reflect.DeepEqual(before, s.Snapshot()) || s.disk.revision != rev {
					t.Fatal("uncertain graph published")
				}
				preserved := outcomeDiskBytes(t, path)
				s.disk.openWrite = nil
				s.disk.readBack = nil
				if e := s.AddToday("must stop"); !errors.Is(e, ErrIncomplete) {
					t.Fatal(e)
				}
				if !reflect.DeepEqual(preserved, outcomeDiskBytes(t, path)) {
					t.Fatal("failure data changed after gate")
				}
			})
		}
	}
	for _, target := range []string{"revision", "root"} {
		t.Run("directory-sync/"+target, func(t *testing.T) {
			s, path, chain, trace := manualServiceFixture(t)
			calls := 0
			s.disk.syncDir = func(root *os.Root, name string) error {
				calls++
				if (target == "root" && name == ".") || (target == "revision" && name != ".") {
					return ErrIncomplete
				}
				return syncDirectory(root, name)
			}
			o := s.SetManualProgressOutcome(chain, trace, 29)
			checkOutcome(t, o.Outcome, o.Receipt != nil)
			if o.Result != ResultUncertain || o.WriteExtent != WriteReservedOrPartial || o.FailureStage != StageSync {
				t.Fatalf("sync: %+v", o)
			}
			want := 1
			if target == "root" {
				want = 2
			}
			if calls != want {
				t.Fatal("sync order", calls)
			}
			preserved := outcomeDiskBytes(t, path)
			s.disk.syncDir = nil
			if e := s.AddToday("must stop"); !errors.Is(e, ErrIncomplete) {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(preserved, outcomeDiskBytes(t, path)) {
				t.Fatal("failed sync data rewritten")
			}
		})
	}
}

func TestOutcomeRawDiskOwnership(t *testing.T) {
	files, err := filepath.Glob("*.go")
	must(t, err)
	found := map[string]int{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		must(t, e)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := outcomeSelectorName(c.Fun)
				if strings.HasPrefix(name, "s.disk.") && (strings.Contains(name, "commit") || strings.Contains(name, "verifyPrefix")) {
					key := path + ":" + fn.Name.Name + ":" + name
					found[key]++
				}
				return true
			})
		}
	}
	want := map[string]int{"outcome.go:commitLocked:s.disk.commitObserved": 1, "outcome.go:verifyPrefixLocked:s.disk.verifyPrefixObserved": 1}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("live Service disk escape: %v", found)
	}
}

func outcomeSelectorName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return outcomeSelectorName(v.X) + "." + v.Sel.Name
	}
	return ""
}

func TestOutcomeLimitReasonIsNotErrorName(t *testing.T) {
	t.Run("revision-capacity-is-not-session-corruption", func(t *testing.T) {
		s, path, _, _ := manualServiceFixture(t)
		// Exercise the store's early hard-cap branch, without constructing 1024
		// expensive real commits or pretending a smaller request removes the cap.
		s.disk.revision = maxRevisions
		before := outcomeDiskBytes(t, path)
		s.mu.Lock()
		a := newAttempt()
		err := s.commitLocked(s.snapshot, &a)
		o := s.outcomeLocked(a, err)
		s.mu.Unlock()
		if !errors.Is(err, ErrLimit) || o.Result != ResultRejected || o.WriteExtent != WriteNone || o.Session != SessionWritable || o.FailureStage != StageCapacity {
			t.Fatalf("capacity: %+v", o)
		}
		if !reflect.DeepEqual(before, outcomeDiskBytes(t, path)) {
			t.Fatal("capacity branch wrote")
		}
	})
	t.Run("existing-prefix-over-entry-limit-invalidates", func(t *testing.T) {
		s, path, _, _ := manualServiceFixture(t)
		// Fault the isolated directory after legitimate domain fixture creation.
		// The prefix enumerator rejects too many entries before name parsing.
		for i := s.disk.revision + 1; i <= maxRevisions+1; i++ {
			must(t, os.Mkdir(filepath.Join(path, fmt.Sprintf("rev-%010d", i)), 0700))
		}
		before := outcomeDiskBytes(t, path)
		o := s.AdvanceToOutcome(s.Today())
		if !errors.Is(o.Err, ErrLimit) || o.Result != ResultRejected || o.WriteExtent != WriteNone || o.Session != SessionNeedsReopen || o.FailureStage != StagePrefix {
			t.Fatalf("prefix limit: %+v", o)
		}
		if !reflect.DeepEqual(before, outcomeDiskBytes(t, path)) {
			t.Fatal("read-only prefix altered")
		}
	})
}

func TestOutcomeCloseFailureStaysClosed(t *testing.T) {
	s, _, chain, trace := manualServiceFixture(t)
	// Inject the otherwise platform-dependent root close error, after closing
	// the actual isolated resource. Service must retain its closed lifecycle.
	injected := errors.New("close failed")
	s.disk.closeRoot = func() error { must(t, s.disk.root.Close()); return injected }
	if err := s.Close(); !errors.Is(err, injected) {
		t.Fatal("original close error lost", err)
	}
	if s.Health().Session != SessionClosed {
		t.Fatal("failed close revived Service")
	}
	o := s.SetManualProgressOutcome(chain, trace, 29)
	if o.Session != SessionClosed || o.Result != ResultRejected || !errors.Is(o.Err, ErrSession) {
		t.Fatalf("closed: %+v", o)
	}
	must(t, s.Close())
}
