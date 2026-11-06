package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Execute the shipped shell scripts against a recording Go stub. This checks
// reachable argv and failure propagation, not merely text that could be dead code.
func TestSelectedLayoutGateContract(t *testing.T) {
	for _, entry := range []string{"check-mygo-native", "check"} {
		for _, mode := range []string{"default", "all-ui", "failure"} {
			if entry == "check" && mode == "all-ui" {
				continue
			}
			t.Run(entry+"/"+mode, func(t *testing.T) {
				dir := t.TempDir()
				write := func(name, content string) {
					t.Helper()
					path := filepath.Join(dir, name)
					if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(content), 0700); err != nil {
						t.Fatal(err)
					}
				}
				for _, name := range []string{"check", "check-mygo-native"} {
					content, err := os.ReadFile("scripts/" + name)
					if err != nil {
						t.Fatal(err)
					}
					write("scripts/"+name, string(content))
				}
				for _, module := range []string{"domain-core", "domain-extensions", "domain-codec", "build"} {
					if err := os.MkdirAll(filepath.Join(dir, module), 0700); err != nil {
						t.Fatal(err)
					}
				}
				write("scripts/environment", "SOURCE_ROOT=\"$FIXTURE\"\nBUILD_ROOT=\"$FIXTURE/build\"\nGO=\"$FIXTURE/go\"\ncd -- \"$SOURCE_ROOT\"\n")
				write("go", "#!/usr/bin/env bash\nprintf '%s\\t' \"$@\" >> \"$TRACE\"\nprintf '\\n' >> \"$TRACE\"\nexit \"${STUB_EXIT:-0}\"\n")
				write("gofmt", "#!/usr/bin/env bash\nexit 0\n")
				write("domain-extensions/classification/canonical16/scripts/verify.sh", "#!/usr/bin/env bash\necho canonical16 >> \"$TRACE\"\n")
				write("scripts/check-dependency", "#!/usr/bin/env bash\necho dependency >> \"$TRACE\"\n")
				args := []string{filepath.Join(dir, "scripts", entry)}
				if mode == "all-ui" {
					args = append(args, "--all-ui")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "bash", args...)
				code := "0"
				if mode == "failure" {
					code = "19"
				}
				trace := filepath.Join(dir, "trace")
				cmd.Env = append(os.Environ(), "FIXTURE="+dir, "TRACE="+trace, "STUB_EXIT="+code)
				output, err := cmd.CombinedOutput()
				data, readErr := os.ReadFile(trace)
				if readErr != nil {
					t.Fatalf("entry did not invoke Go: %v; output=%s", readErr, output)
				}
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				if mode == "failure" {
					var failure *exec.ExitError
					if !errors.As(err, &failure) || failure.ExitCode() != 19 || len(lines) != 1 {
						t.Fatalf("failure did not propagate immediately: err=%v trace=%s output=%s", err, data, output)
					}
					return
				}
				if err != nil {
					t.Fatalf("%v: %s", err, output)
				}
				want := 3
				if entry == "check" {
					want = 13
				}
				if len(lines) != want {
					t.Fatalf("incomplete execution: got %d calls, want %d: %s", len(lines), want, data)
				}
				if entry == "check" && (lines[8] != "canonical16" || lines[9] != "dependency") {
					t.Fatalf("missing prerequisite gates: %s", data)
				}
				native := lines[len(lines)-3:]
				argv := strings.Fields(native[0])
				if len(argv) != 9 || strings.Join(argv[:7], " ") != "test -p=1 -race -count=1 -buildvcs=false github.com/egoist/mygo/ui -run" || argv[8] != "-v" {
					t.Fatalf("wrong native test argv: %q", argv)
				}
				pattern := regexp.MustCompile(argv[7])
				for _, name := range []string{"TestLayoutInputs", "TestLayoutCacheMatches", "TestLayoutCacheReuses", "TestLayoutCacheInvalidates", "TestNoonmarkSelectedLayoutCache", "TestNoonmarkSelectedButtonIndependentState"} {
					if !pattern.MatchString(name) {
						t.Errorf("default native gate skips %s", name)
					}
				}
				if mode == "all-ui" && argv[7] != "." {
					t.Fatal("all-ui filtered tests")
				}
				if strings.Join(strings.Fields(native[1]), " ") != "vet -p=1 -buildvcs=false github.com/egoist/mygo/ui github.com/egoist/mygo" {
					t.Fatalf("missing vet: %s", native[1])
				}
				if strings.Join(strings.Fields(native[2]), " ") != "test -p=1 -race -count=1 -buildvcs=false github.com/egoist/mygo -run ^TestNoonmarkContentPopupCompletion$ -v" {
					t.Fatalf("missing host boundary: %s", native[2])
				}
			})
		}
	}
}

// A successful go test exit with no matching tests is not a symptom pass.
func TestSelectedLayoutDependencySymptoms(t *testing.T) {
	goTool := os.Getenv("NOONMARK_GO")
	if goTool == "" {
		goTool = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goTool, "test", "-p=1", "-race", "-count=1", "-buildvcs=false", "-json", "github.com/egoist/mygo/ui", "-run", "^Test(LayoutInputs|NoonmarkSelectedLayoutCache)$")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepared dependency symptoms: %v\n%s", err, output)
	}
	want := map[string]bool{"TestLayoutInputs": false, "TestNoonmarkSelectedLayoutCache": false}
	for _, role := range []string{"button", "tab", "tree_item", "row", "list_item"} {
		want["TestNoonmarkSelectedLayoutCache/"+role] = false
	}
	ran := map[string]bool{}
	for _, line := range bytes.Split(output, []byte("\n")) {
		var event struct{ Action, Package, Test string }
		if json.Unmarshal(line, &event) != nil || event.Package != "github.com/egoist/mygo/ui" {
			continue
		}
		if _, ok := want[event.Test]; !ok {
			continue
		}
		switch event.Action {
		case "run":
			ran[event.Test] = true
		case "pass":
			want[event.Test] = true
		case "skip", "fail":
			t.Fatalf("symptom %s: %s", event.Test, event.Action)
		}
	}
	for name, passed := range want {
		if !ran[name] || !passed {
			t.Errorf("symptom did not run and pass: %s\n%s", name, output)
		}
	}
}

func TestSelectedLayoutPatchScope(t *testing.T) {
	const dir = "patches/mygo/native-finalize/"
	data, err := os.ReadFile(dir + "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Patches []struct {
			File  string
			Files map[string]json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, patch := range manifest.Patches {
		if patch.File != "0007-selected-layout-regression-tests.patch" {
			continue
		}
		found = true
		if len(patch.Files) != 2 || patch.Files["ui/layoutcache_test.go"] == nil || patch.Files["ui/noonmark_selected_layout_test.go"] == nil {
			t.Fatal("selected layout repair must remain test-only")
		}
		content, err := os.ReadFile(dir + patch.File)
		if err != nil {
			t.Fatal(err)
		}
		for _, assertion := range []string{"checked selected expanded", "func TestNoonmarkSelectedLayoutCache", "fresh.rt.noLayoutCache = true", "sameBoxes(", "reflect.DeepEqual(cached.h.access, fresh.h.access)", "n.Selected() != on", "cached.rt.computed - before"} {
			if !strings.Contains(string(content), assertion) {
				t.Errorf("missing selected layout regression assertion: %s", assertion)
			}
		}
	}
	if !found {
		t.Fatal("default manifest omits selected layout repair")
	}
}
