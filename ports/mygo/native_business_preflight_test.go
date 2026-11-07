package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func businessArgs(parent string) []string {
	return append(smokeArgs(parent), "-runtime-profile=ordinary-development", "-native-business-scenario=ordinary-category-v1")
}
func TestNativeBusinessArgumentsPure(t *testing.T) {
	root := t.TempDir()
	good := businessArgs(root)
	for _, args := range [][]string{
		{"-native-business-scenario=ordinary-category-v1"},
		append(smokeArgs(root), "-native-business-scenario=ordinary-category-v1"),
		append(smokeArgs(root), "-runtime-profile=legacy-development", "-native-business-scenario=ordinary-category-v1"),
		append(smokeArgs(root), "-runtime-profile=ordinary-development", "-native-business-scenario="),
		append(smokeArgs(root), "-runtime-profile=ordinary-development", "-native-business-scenario=unknown"),
		append(append([]string{}, good...), "-native-business-scenario=ordinary-category-v1"),
		append(append([]string{}, good...), "-prototype=false"),
		append(append([]string{}, good...), "-render-one=export"),
		append(append([]string{}, good...), "-list=false"),
		append(append([]string{}, good...), "-screen=pool"),
		append(append([]string{}, good...), "--"),
		append(append([]string{}, good...), "tail"),
	} {
		before := startupTree(t, root)
		if _, e := parseRuntimeStartup(args); e == nil {
			t.Fatalf("accepted %q", args)
		}
		if !reflect.DeepEqual(before, startupTree(t, root)) {
			t.Fatal("parse wrote")
		}
	}
	before := startupTree(t, root)
	p, e := planNativeWindowReport(good)
	if e != nil || p.businessBase != "stage1-business.json" {
		t.Fatalf("plan: %v %v", p, e)
	}
	if !reflect.DeepEqual(before, startupTree(t, root)) {
		t.Fatal("plan wrote")
	}
}
func TestNativeBusinessReportPlanAllDestinations(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "stage1-business.json")
			var err error
			switch kind {
			case "file":
				err = os.WriteFile(target, []byte("keep"), 0600)
			case "directory":
				err = os.Mkdir(target, 0700)
			case "symlink":
				err = os.Symlink(filepath.Join(root, "absent"), target)
			}
			if err != nil {
				t.Skip(err)
			}
			if _, err = planNativeWindowReport(businessArgs(root)); err == nil {
				t.Fatal("accepted occupied business destination")
			}
			if _, err = os.Lstat(filepath.Join(root, "stage1.json")); !os.IsNotExist(err) {
				t.Fatal("created first report during planning")
			}
		})
	}
}
func TestNativeBusinessSecondExclusiveCreateFailurePreservesFirst(t *testing.T) {
	root := t.TempDir()
	p, e := planNativeWindowReport(businessArgs(root))
	if e != nil {
		t.Fatal(e)
	}
	first, e := createNativeWindowReport(p)
	if e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(root, p.businessBase)
	if e = os.WriteFile(target, []byte("winner"), 0600); e != nil {
		t.Fatal(e)
	}
	if f, e := createNativeBusinessReport(p); e == nil || f != nil {
		t.Fatal("lost exclusivity")
	}
	_ = first.WriteFinal(nil)
	if info, e := os.Stat(filepath.Join(root, p.base)); e != nil || info.Size() != 0 {
		t.Fatal("first report not preserved")
	}
	if data, e := os.ReadFile(target); e != nil || string(data) != "winner" {
		t.Fatal("overwrote winner")
	}
	if _, e := os.Lstat(filepath.Join(root, "session")); !os.IsNotExist(e) {
		t.Fatal("created session")
	}
}
func TestNativeBusinessOpenReportAndLegacyPlan(t *testing.T) {
	root := t.TempDir()
	if e := os.Mkdir(filepath.Join(root, "session"), 0700); e != nil {
		t.Fatal(e)
	}
	args := []string{"-session", filepath.Join(root, "session"), "-native-window-smoke", filepath.Join(root, "stage2.json"), "-runtime-profile=ordinary-development", "-native-business-scenario=ordinary-category-v1"}
	p, e := planNativeWindowReport(args)
	if e != nil || p.businessBase != "stage2-business.json" {
		t.Fatal(p, e)
	}
	p, e = planNativeWindowReport(args[:5])
	if e != nil || p.businessBase != "" {
		t.Fatal("legacy plan changed", p, e)
	}
}
