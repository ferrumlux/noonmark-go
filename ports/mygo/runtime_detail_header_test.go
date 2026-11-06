// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskview"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDetailHeaderOriginalSymptom(t *testing.T) {
	f := newTitleFixture(t, true)
	if _, ok := f.tt.Find("关闭任务详情"); !ok {
		t.Error("missing independent xmark")
	}
	if _, ok := f.tt.Find("返回日统计"); ok {
		t.Error("extra return row")
	}
	date, ok := f.tt.AccessByRole(ui.RoleText, "日任务详情日期")
	if !ok || date.Value != "11月5日 周四" {
		t.Errorf("metadata date does not match Swift formatter: %+v", date)
	}
}
func TestDetailHeaderSourceGeometry(t *testing.T) {
	g := detailMetadataLayout(252, 33, 16, 85, 16)
	if g.chipW != 59 || g.chipH != 20 || g.radius != 10 || g.dateX != 67 || g.dateY != 2 || g.chipY != 0 || g.height != 20 {
		t.Fatal(g)
	}
	wrap := detailMetadataLayout(151, 33, 16, 85, 16)
	if wrap.dateX != 0 || wrap.dateY != 28 || wrap.height != 44 {
		t.Fatal(wrap)
	}
	tall := detailMetadataLayout(300, 33, 18, 100, 24)
	if tall.chipH != 22 || tall.chipY != 1 || tall.height != 24 || tall.dateY != 0 {
		t.Fatal(tall)
	}
	for i, d := range []core.LocalDate{{Year: 2026, Month: 11, Day: 1}, {Year: 2026, Month: 11, Day: 2}, {Year: 2026, Month: 11, Day: 3}, {Year: 2026, Month: 11, Day: 4}, {Year: 2026, Month: 11, Day: 5}, {Year: 2026, Month: 11, Day: 6}, {Year: 2026, Month: 11, Day: 7}} {
		if got := detailDateText(d); got != fmt.Sprintf("11月%d日 周%s", i+1, []string{"日", "一", "二", "三", "四", "五", "六"}[i]) {
			t.Fatal(got)
		}
	}
}
func TestDetailHeaderStatusColorsAndHiddenCancellation(t *testing.T) {
	cases := []struct {
		s           core.TraceStatus
		fg, bg, dot ui.Color
	}{
		{core.TracePending, sourceVisual.accent, sourceVisual.accentSoft, sourceVisual.accent},
		{core.TraceCompleted, sourceVisual.success, sourceRGB(.91, .98, .95), sourceVisual.success},
		{core.TraceUnfinished, sourceVisual.warning, sourceRGB(1, .937, .922), sourceVisual.warning},
		{core.TraceDeferred, sourceVisual.text2, sourceVisual.chip, sourceVisual.text3},
		{core.TraceChanged, sourceVisual.text2, sourceVisual.chip, sourceVisual.text3},
		{core.TraceReturnedToPool, sourceVisual.text2, sourceVisual.chip, sourceVisual.text3},
		{core.TraceAbandoned, sourceVisual.text3, sourceVisual.chip, sourceVisual.text3},
	}
	for _, x := range cases {
		style, ok := detailStatusStyle(x.s)
		if !ok || style != (detailStatusColors{x.fg, x.bg, x.dot}) {
			t.Fatal(x.s, style)
		}
	}
	for _, s := range []core.TraceStatus{core.TraceCancelledDraft, "unknown"} {
		if _, ok := detailStatusStyle(s); ok {
			t.Fatal("internal/unknown visible", s)
		}
		tt := ui.NewTester(func(c *ui.Context) {
			liveDetailMetadata(c, 252, taskview.DayRow{CalendarRow: taskview.CalendarRow{Status: s}}, liveTestDay)
		}, 252, 70)
		if _, ok := tt.Find("日任务详情状态"); ok {
			t.Fatal("hidden status rendered")
		}
	}
}
func detailSavePNG(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	root := os.Getenv("NOONMARK_MYGO_BUILD_ROOT")
	if root == "" {
		t.Fatal("external root required")
	}
	dir := filepath.Join(root, "evidence", "day-detail")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	f, e := os.Create(filepath.Join(dir, name))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e = png.Encode(f, tt.Image()); e != nil {
		t.Fatal(e)
	}
}
func TestDetailHeaderMetadataPixelsAndWrap(t *testing.T) {
	for _, width := range []float32{252, 125} {
		for _, scale := range []float32{1, 2} {
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Size(width, 100).Background(sourceVisual.panel).Children(func() {
					liveDetailMetadata(c, width, taskview.DayRow{CalendarRow: taskview.CalendarRow{Status: core.TracePending}}, liveTestDay)
				})
			}, int(width), 100)
			tt.SetScale(scale)
			status, ok := tt.Find("日任务详情状态")
			if !ok {
				t.Fatal("status")
			}
			date, ok := tt.Find("日任务详情日期")
			if !ok {
				t.Fatal("date")
			}
			h := status.H + 4
			if status.X != 18 || status.Y != 2 {
				t.Fatal("status padding", status)
			}
			if width == 252 {
				if math.Abs(float64(date.X-(status.X+status.W+8+8))) > .01 || math.Abs(float64(date.Y+date.H/2-h/2)) > .01 {
					t.Fatal("not centred same row", status, date)
				}
			} else if date.X != 0 || date.Y != h+8 {
				t.Fatal("not whole-date wrapped", status, date)
			}
			im := tt.Image()
			progressPixel(t, im, int(10.5*scale), int(h/2*scale), sourceVisual.accent)
			progressPixel(t, im, int(16*scale), int(2*scale), sourceVisual.accentSoft)
			progressPixel(t, im, 0, 0, sourceVisual.panel)
			detailSavePNG(t, tt, fmt.Sprintf("metadata-%d-%dx.png", int(width), int(scale)))
		}
	}
}
func TestDetailHeaderServiceVisualMatrix(t *testing.T) {
	for _, state := range []string{"pending", "completed", "unfinished"} {
		t.Run(state, func(t *testing.T) {
			f := newTitleFixture(t, false)
			if err := f.svc.AddToday(strings.Repeat("合成长标题与多子任务保留阅读空间。", 6)); err != nil {
				t.Fatal(err)
			}
			tr := f.svc.Snapshot().Traces[0]
			for i := 0; i < 12; i++ {
				id, e := f.svc.AddSubtask(tr.ChainID, tr.ID, fmt.Sprintf("合成子任务 %02d：检查详情顶部与滚动", i+1), core.DifficultyMedium)
				if e != nil {
					t.Fatal(e)
				}
				if e = f.svc.SetSubtaskCompleted(tr.ChainID, tr.ID, id, true); e != nil {
					t.Fatal(e)
				}
			}
			if state == "completed" {
				if e := f.svc.SetCompleted(tr.ID, true); e != nil {
					t.Fatal(e)
				}
			}
			if state != "pending" {
				next, e := liveTestDay.AddDays(1)
				if e != nil {
					t.Fatal(e)
				}
				if e = f.svc.AdvanceTo(next); e != nil {
					t.Fatal(e)
				}
			}
			f.h.publish(f.svc.Snapshot(), f.svc.Today())
			f.s.runtimeDate = liveTestDay
			f.s.runtimeSelection = string(tr.ID)
			f.s.runtimeDayRail = true
			f.tt.Frame()
			f.ui()
			before := f.svc.Snapshot()
			for _, size := range [][2]int{{1200, 768}, {960, 720}} {
				for _, scale := range []float32{1, 2} {
					f.tt.SetSize(size[0], size[1])
					f.tt.SetScale(scale)
					f.tt.Frame()
					f.ui()
					close, ok := f.tt.Find("关闭任务详情")
					if !ok {
						t.Fatal("close missing")
					}
					pane, _ := f.tt.Find("收起日待办详情")
					if close.W != 28 || close.H != 28 || pane.X-close.X-close.W != 8 || close.Y != pane.Y {
						t.Fatal("header targets overlap", close, pane)
					}
					date, _ := f.tt.Find("日任务详情日期")
					status, _ := f.tt.Find("日任务详情状态")
					if math.Abs(float64(date.Y+date.H/2-status.Y-status.H/2)) > .01 {
						t.Fatal("metadata not centred", date, status)
					}
					if state != "pending" {
						if _, ok := f.tt.AccessByRole(ui.RoleTextField, "日任务详情标题"); ok {
							t.Fatal("history writable")
						}
					}
					if _, ok := f.tt.AccessByRole(ui.RoleSlider, ""); ok {
						t.Fatal("invented slider")
					}
					detailSavePNG(t, f.tt, fmt.Sprintf("%s-%dx%d-%dx.png", state, size[0], size[1], int(scale)))
				}
			}
			if !reflect.DeepEqual(before, f.svc.Snapshot()) {
				t.Fatal("render mutated Service")
			}
		})
	}
}

func TestDetailHeaderServiceShortCapture(t *testing.T) {
	f, tr, _ := subtaskFixture(t)
	liveClick(t, f.tt, "展开子任务 "+string(tr.ID))
	f.all()
	f.tt.SetScale(2)
	f.tt.Frame()
	detailSavePNG(t, f.tt, "short-pending-1200x768-2x.png")
}
