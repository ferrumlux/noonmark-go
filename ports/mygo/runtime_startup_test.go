// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

func startupTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			out[rel] = "directory"
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestRuntimeStartupInvalidArgumentsArePure(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "session")
	report := filepath.Join(root, "stage1.json")
	render := filepath.Join(root, "render")
	base := []string{"-runtime-profile=ordinary-development", "-init-session", p}
	cases := [][]string{
		{"-runtime-profile=unknown", "-init-session", p}, {"-runtime-profile=", "-init-session", p}, {"-runtime-profile"},
		{"-runtime-profile=ordinary-development"}, {"-init-session", p, "-session", p},
		append(append([]string{}, base...), "-runtime-profile", "ordinary-development"),
		append(append([]string{}, base...), "-screen=day", "-screen", "pool"),
		append(append([]string{}, base...), "-unknown"), append(append([]string{}, base...), "extra"),
		append(append([]string{}, base...), "--", "tail"), append(append([]string{}, base...), "-prototype"),
		append(append([]string{}, base...), "-list"), append(append([]string{}, base...), "-render", render),
		append(append([]string{}, base...), "-render-one", render), {"-list", "-unknown"}, {"-list", "tail"},
		{"-render-one", render, "-screen=no-such-screen"}, {"-prototype=wrong"}, {"-screen="}, {"-screen"},
	}
	for i, args := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			before := startupTree(t, root)
			if _, err := parseRuntimeStartup(args); err == nil {
				t.Fatalf("accepted %q", args)
			}
			smoke := append(append([]string{}, args...), "-native-window-smoke", report)
			if f, err := nativeWindowPreflight(smoke); err == nil || f != nil {
				t.Fatalf("smoke accepted %q", smoke)
			}
			if !reflect.DeepEqual(before, startupTree(t, root)) {
				t.Fatal("invalid args wrote files")
			}
		})
	}
}
func TestRuntimeStartupPathRefusalIsPure(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing")
	if err := os.Mkdir(existing, 0700); err != nil {
		t.Fatal(err)
	}
	paths := []string{"relative", root + "/existing/../new", existing}
	link := filepath.Join(root, "link")
	if err := os.Symlink(existing, link); err == nil {
		paths = append(paths, filepath.Join(link, "session"))
	}
	for _, p := range paths {
		o, err := parseRuntimeStartup([]string{"-runtime-profile=ordinary-development", "-init-session", p})
		if err != nil {
			t.Fatal(err)
		}
		if err = o.checkPaths(); err == nil {
			t.Fatalf("accepted unsafe path %q", p)
		}
	}
}
func TestRuntimeStartupOrdinaryFactoryColdOpen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session")
	o, err := parseRuntimeStartup([]string{"-runtime-profile=ordinary-development", "-init-session", p})
	if err != nil {
		t.Fatal(err)
	}
	if err = o.checkPaths(); err != nil {
		t.Fatal(err)
	}
	s, err := o.openService(liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AddToday("真实完整任务服务"); err != nil {
		t.Fatal(err)
	}
	chain := s.Snapshot().Chains[0].ID
	result := s.ReplaceCategorySelectionOn(liveTestDay, chain, ordinary.Choice{Kind: ordinary.ChoiceNew, Name: "工程", ColorHex: "#D87831"}, ordinary.UUID("10000000-0000-4000-8000-000000000001"))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	before, err := s.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	o.initSession = ""
	o.session = p
	disk := startupTree(t, p)
	s, err = o.openService(liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(disk, startupTree(t, p)) {
		t.Fatal("same-day cold open changed facts or disk")
	}
	s.Close()
	next, _ := liveTestDay.AddDays(1)
	s, err = o.openService(next)
	if err != nil {
		t.Fatal(err)
	}
	advanced, err := s.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if advanced.StoreRevision != before.StoreRevision+1 || len(advanced.Classification.Categories) != 1 {
		t.Fatalf("unexpected day advance %d -> %d", before.StoreRevision, advanced.StoreRevision)
	}
	s.Close()
	disk = startupTree(t, p)
	s, err = o.openService(next)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if !reflect.DeepEqual(disk, startupTree(t, p)) {
		t.Fatal("second open repeated day advance")
	}
	o.profile = legacyDevelopment
	if s, err = o.openService(next); err == nil || s != nil {
		t.Fatal("legacy opened nonempty ordinary")
	}
	if !reflect.DeepEqual(disk, startupTree(t, p)) {
		t.Fatal("legacy refusal changed disk")
	}
}
func TestRuntimeStartupLegacyEmptyAndFailureTruth(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session")
	s, err := taskapp.Create(p, liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	o := runtimeStartup{profile: ordinaryDevelopment, session: p}
	s, err = o.openService(liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, stage := range []string{"会话启动", "窗口初始化"} {
		msg := o.startupFailure(stage, errors.New("fault")).Error()
		if !strings.Contains(msg, "可能已写入") || !strings.Contains(msg, "目录已保留") || strings.Contains(msg, "零写") {
			t.Fatal(msg)
		}
	}
}

func TestRuntimeStartupHubBeforeTwoWindows(t *testing.T) {
	o := runtimeStartup{profile: ordinaryDevelopment, initSession: filepath.Join(t.TempDir(), "session")}
	s, err := o.openService(liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.AddPool("完整任务服务"); err != nil {
		t.Fatal(err)
	}
	var lane, posts titleTestQueue
	var clock titleTestClock
	h := newTitleHub(s, &lane, posts.Enqueue, &clock, clock.Now, func() {})
	if err = h.enableCategoryCandidate(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		a := newRuntimeApp(h.readView())
		h.attach(a, func() {}, func() {})
	}
	if len(h.windows) != 2 || h.category == nil || h.store != s {
		t.Fatal("windows do not share admitted Service")
	}
	if err = h.enableCategoryCandidate(); err == nil {
		t.Fatal("enabled after windows attached")
	}
}

func TestRuntimeStartupMainRoutingOrder(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "func main()")
	ordered := []string{"parseRuntimeStartup(os.Args[1:])", "startup.checkPaths()", "planNativeWindowReport(os.Args[1:])", "flag.Parse()", "createNativeWindowReport(reportPlan)", "startup.openService(today)", "hub = newTitleHub(", "hub.enableCategoryCandidate()", "mygo.NewWindow(", "hub.attach("}
	for _, part := range ordered {
		i := strings.Index(s[start:], part)
		if i < 0 {
			t.Fatalf("missing or reordered startup step %s", part)
		}
		start += i + len(part)
	}
	if strings.Contains(s[strings.Index(s, "func main()"):], "taskapp.Create(") || strings.Contains(s[strings.Index(s, "func main()"):], "taskapp.Open(") {
		t.Fatal("main bypasses unified factory")
	}
}

// A real two-process reserve race, separate from simulated UI windows.
func TestRuntimeStartupProcessWriter(t *testing.T) {
	p := os.Getenv("NOONMARK_ENTRY_TEST_SESSION")
	if p == "" {
		return
	}
	gate := os.Getenv("NOONMARK_ENTRY_TEST_GATE")
	id := os.Getenv("NOONMARK_ENTRY_TEST_ID")
	o := runtimeStartup{profile: ordinaryDevelopment, session: p}
	s, err := o.openService(liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = os.WriteFile(gate+".ready"+id, []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err = os.Stat(gate); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gate timeout")
		}
		time.Sleep(time.Millisecond * 5)
	}
	result := "confirmed"
	if err = s.AddPool("process " + id); err != nil {
		result = "rejected"
		if err = s.AddPool("must remain rejected"); err == nil {
			t.Fatal("loser kept stale write authority")
		}
	}
	if err = os.WriteFile(gate+".result"+id, []byte(result), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestRuntimeStartupTwoProcessReserve(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "session")
	gate := filepath.Join(root, "gate")
	o := runtimeStartup{profile: ordinaryDevelopment, initSession: p}
	s, err := o.openService(liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	commands := []*exec.Cmd{}
	for _, id := range []string{"a", "b"} {
		c := exec.Command(exe, "-test.run=^TestRuntimeStartupProcessWriter$", "-test.timeout=15s")
		c.Env = append(os.Environ(), "NOONMARK_ENTRY_TEST_SESSION="+p, "NOONMARK_ENTRY_TEST_GATE="+gate, "NOONMARK_ENTRY_TEST_ID="+id)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err = c.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, c)
		t.Cleanup(func() {
			if c.ProcessState == nil {
				_ = c.Process.Kill()
				_ = c.Wait()
			}
		})
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, a := os.Stat(gate + ".readya")
		_, b := os.Stat(gate + ".readyb")
		if a == nil && b == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("process readiness timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err = os.WriteFile(gate, []byte("go"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, c := range commands {
		if err = c.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	confirmed := 0
	for _, id := range []string{"a", "b"} {
		b, err := os.ReadFile(gate + ".result" + id)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) == "confirmed" {
			confirmed++
		}
	}
	if confirmed != 1 {
		t.Fatalf("confirmed writers %d", confirmed)
	}
	o.initSession = ""
	o.session = p
	s, err = o.openService(liveTestDay)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Snapshot().Chains) != 1 {
		t.Fatal("race wrote more than one task")
	}
}

func TestRuntimeStartupReportPlanHasNoSideEffects(t *testing.T) {
	root := t.TempDir()
	args := []string{"-runtime-profile=ordinary-development", "-init-session", filepath.Join(root, "session"), "-native-window-smoke", filepath.Join(root, "stage1.json")}
	before := startupTree(t, root)
	plan, err := planNativeWindowReport(args)
	if err != nil {
		t.Fatal(err)
	}
	if plan == nil || !reflect.DeepEqual(before, startupTree(t, root)) {
		t.Fatal("read-only preflight wrote")
	}
	if err = os.WriteFile(filepath.Join(root, "stage1.json"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	before = startupTree(t, root)
	if f, err := createNativeWindowReport(plan); err == nil || f != nil {
		t.Fatal("report race overwrote existing file")
	}
	if !reflect.DeepEqual(before, startupTree(t, root)) {
		t.Fatal("failed exclusive create changed files")
	}
}

// Explicit test-only fixture producer for external real-binary cold-open checks.
// The product CLI has no hidden seed/debug bypass.
func TestRuntimeStartupCLISeedFixture(t *testing.T) {
	root := os.Getenv("NOONMARK_ENTRY_TEST_SEED_ROOT")
	if root == "" {
		return
	}
	o := runtimeStartup{profile: ordinaryDevelopment, initSession: filepath.Join(root, "session")}
	if err := o.checkPaths(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	day := core.LocalDate{Year: now.Year(), Month: int(now.Month()), Day: now.Day()}
	s, err := o.openService(day)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.AddToday("合成 CLI 冷开任务"); err != nil {
		t.Fatal(err)
	}
	out := s.ReplaceCategorySelectionOn(day, s.Snapshot().Chains[0].ID, ordinary.Choice{Kind: ordinary.ChoiceNew, Name: "合成普通分类", ColorHex: "#D87831"}, ordinary.UUID("10000000-0000-4000-8000-000000000002"))
	if out.Err != nil || out.Receipt == nil {
		t.Fatalf("seed selection: %+v", out)
	}
}
