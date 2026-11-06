// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func navigationBadges(t *testing.T, s *app, compact bool) map[string]int {
	t.Helper()
	width := float32(220)
	if compact {
		width = 72
	}
	tt := ui.NewTester(func(c *ui.Context) { s.liveSidebar(c, 768, width, s.runtime.Snapshot()) }, int(width), 768)
	got := map[string]int{}
	page := ""
	for _, text := range tt.Texts() {
		for _, n := range nav {
			if text == n.title {
				page = n.page
			}
		}
		if count, err := strconv.Atoi(text); err == nil {
			if page == "" {
				t.Fatal("unowned sidebar number", text)
			}
			got[page] = count
		}
	}
	return got
}
func wantNavigationBadges(t *testing.T, s *app, want map[string]int) {
	t.Helper()
	if got := navigationBadges(t, s, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("native navigation badges = %v; want %v", got, want)
	}
}
func navigationSavePNG(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
	if root == "" {
		t.Fatal("explicit evidence root required")
	}
	dir := filepath.Join(root, "evidence", "navigation-counts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	phase := os.Getenv("NOONMARK_NAVIGATION_EVIDENCE_PHASE")
	if phase == "" {
		phase = "green"
	}
	f, err := os.Create(filepath.Join(dir, phase+"-"+name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
func TestNavigationUIEmptyHidesZero(t *testing.T) {
	f := newTitleFixture(t, false)
	f.tt.SetScale(2)
	navigationSavePNG(t, f.tt, "empty-2x.png")
	wantNavigationBadges(t, f.s, map[string]int{})
}
func TestNavigationUITodayOnlyPending(t *testing.T) {
	f := newTitleFixture(t, true)
	tr := f.svc.Snapshot().Traces[0]
	liveClick(t, f.tt, "切换完成 "+string(tr.ID))
	f.all()
	if f.svc.Snapshot().Traces[0].Status != core.TraceCompleted {
		t.Fatal("real Service completion missing")
	}
	f.tt.SetScale(2)
	navigationSavePNG(t, f.tt, "completed-today-2x.png")
	wantNavigationBadges(t, f.s, map[string]int{"completed": 1})
	liveClick(t, f.tt, "切换完成 "+string(tr.ID))
	f.all()
	wantNavigationBadges(t, f.s, map[string]int{"day": 1})
}
func TestNavigationUICompletedChildCountsChain(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	if f.svc.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("fixture parent must be pending")
	}
	wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
	f.tt.SetScale(2)
	navigationSavePNG(t, f.tt, "completed-child-2x.png")
	wantNavigationBadges(t, f.s, map[string]int{"day": 1, "completed": 1})
}
