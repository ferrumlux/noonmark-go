// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func TestReviewProgressUIDeepSnapshotsAndEqualPublication(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	g := f.svc.Snapshot()
	manual, desc, review := 20, "合成描述", "合成总结"
	g.Traces[0].ManualProgressPercent = &manual
	g.Traces[0].DescriptionText = &desc
	g.Days[0].ReviewSummary = &review
	g.Traces[0].NoteEntries = []core.TaskNoteEntry{{ID: core.TaskNoteEntryID(progressNewID(t)), Body: "合成备注", CreatedAt: tr.CreatedAt, UpdatedAt: tr.CreatedAt}}
	if err := core.ValidateTaskGraph(g); err != nil {
		t.Fatal(err)
	}
	f.h.publish(g, liveTestDay)
	f.ui()
	before, projection := f.h.graph.Clone(), f.h.progress.projection
	f.h.publish(g.Clone(), liveTestDay)
	f.ui()
	if projection != f.h.progress.projection {
		t.Fatal("deep-equal allocated snapshot rebuilt projection")
	}
	*g.Traces[0].ManualProgressPercent = 99
	*g.Traces[0].DescriptionText = "外部变更"
	*g.Days[0].ReviewSummary = "外部变更"
	g.Traces[0].NoteEntries[0].Body = "外部变更"
	g.Subtasks[0].Difficulty = core.DifficultyHard
	exposed := f.h.readView().Snapshot()
	*exposed.Traces[0].DescriptionText = "读取副本变更"
	exposed.Subtasks[1].Status = core.SubtaskCompleted
	if !reflect.DeepEqual(before, f.h.graph) || !reflect.DeepEqual(before, f.h.progress.graph) {
		t.Fatal("publication retains mutable external data")
	}
	wantUIProgress(t, f.s, tr.ID, 25, 1, 4)
	if projection != f.h.progress.projection {
		t.Fatal("external mutation rebuilt immutable projection")
	}
}

func TestReviewProgressUISameDaySequenceAndHistoricalMax(t *testing.T) {
	f, tr, easy, _ := progressFixtureUI(t)
	g := progressPriorManual(t, f.svc.Snapshot(), 80)
	// Lower-sequence pending current row and a later completed same-day peer.
	// The later 100 floor cannot leak backward; same-day raw lineages still count.
	g.Traces[0].CarriedFromTraceID = nil
	g.Traces[0].ContinuationSeq = 0
	peer := g.Traces[0]
	peer.ID = core.DayTraceID(progressNewID(t))
	peer.Status = core.TraceCompleted
	peer.Priority = 2
	peer.CompletedAt = &peer.CreatedAt
	peer.CarriedFromTraceID = &g.Traces[1].ID
	peer.ContinuationSeq = 1
	g.Traces = append(g.Traces, peer)
	historic := easy
	historic.ID = core.SubtaskID(progressNewID(t))
	historic.TraceID = peer.ID
	historic.Difficulty = core.DifficultyHard
	g.Subtasks = append(g.Subtasks, historic)
	if err := core.ValidateTaskGraph(g); err != nil {
		t.Fatal(err)
	}
	f.h.publish(g, liveTestDay)
	f.ui()
	p := wantUIProgress(t, f.s, tr.ID, 80, 3, 6)
	if p.FloorPercent != 80 || p.Mode != core.ProgressWeightedSubtasks {
		t.Fatal(p)
	}
	wantUIProgress(t, f.s, peer.ID, 100, 1, 1)
	// Current count is one of two, but historical maximum is three of six,
	// displayed as 80 due the earlier day's manual floor.
	progressAccess(t, f.tt, core.ProgressWeightedSubtasks, 80)
	f.tt.SetScale(2)
	f.all()
	progressSavePNG(t, f.tt, "review-same-day-max-and-floor-2x.png")
	if !reflect.DeepEqual(f.svc.Snapshot().Traces[0], tr) {
		t.Fatal("synthetic review graph persisted")
	}
}

func TestReviewProgressUIPublicationPreservesMarkedParentAndChild(t *testing.T) {
	for _, target := range []string{"parent", "child"} {
		t.Run(target, func(t *testing.T) {
			f, tr, easy, hard := progressFixtureUI(t)
			other := newRuntimeApp(f.h.readView())
			f.h.attach(other, func() {}, func() {})
			label := "日任务详情标题"
			if target == "child" {
				label = subtaskPrefix("day", string(easy.ID)) + ".title"
			}
			liveClick(t, f.tt, label)
			f.tt.Key(ui.Cmd, ui.KeyA)
			titleDirectClient(t, f.tt).SetMarkedText(nil, "保留候选", ui.TextInputRange{Start: 4, End: 4})
			f.ui()
			before := f.h.progress.projection
			f.h.command(other, func() error { return f.svc.SetSubtaskDifficulty(tr.ChainID, tr.ID, hard.ID, core.DifficultyMedium) }, false, nil)
			f.all()
			wantUIProgress(t, f.s, tr.ID, 33, 1, 3)
			if before == f.h.progress.projection {
				t.Fatal("changed graph not published")
			}
			e := f.s.titleEditor
			if target == "child" {
				e = f.s.subtaskEditors[subtaskOwner("day", string(easy.ID))]
			}
			if e == nil {
				t.Fatal("editor lost")
			}
			snap, ok := e.native.handle.TextInputSnapshot(e.native.context)
			if !ok || !strings.Contains(snap.Text+snap.MarkedText, "保留候选") {
				t.Fatal("progress publication dropped preedit", snap, ok)
			}
			f.advance(time.Second)
			f.all()
			if f.persisted() != "原题" || subtaskPersisted(t, f, easy.ID).Title != "原子任务" {
				t.Fatal("progress publication persisted preedit")
			}
			titleDirectClient(t, f.tt).ReplaceText(nil, "确认文本")
			f.tt.Frame()
			f.ui()
			f.advance(700 * time.Millisecond)
			f.all()
			got := f.persisted()
			if target == "child" {
				got = subtaskPersisted(t, f, easy.ID).Title
			}
			if got != "确认文本" {
				t.Fatal("confirmed text not saved", got)
			}
		})
	}
}

func TestReviewProgressUIScrollAndFullOverlayHitTarget(t *testing.T) {
	f, tr, _, _ := progressFixtureUI(t)
	for i := 0; i < 32; i++ {
		if err := f.svc.AddToday(fmt.Sprintf("滚动隔离父项%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.s.titleTransition(func() { f.s.runtimeDayRail = false; f.s.runtimeSelection = "" })
	f.all()
	f.tt.SetSize(960, 720)
	f.tt.SetScale(2)
	f.all()
	before := f.h.progress.projection
	strip, _ := f.tt.Find("日条选择日期 2026-11-05")
	f.tt.Scroll(700, 500, 0, 1700)
	f.all()
	after, _ := f.tt.Find("日条选择日期 2026-11-05")
	if strip != after {
		t.Fatal("progress rows moved fixed date strip")
	}
	f.tt.Scroll(700, 500, 0, -3000)
	f.all()
	bar, ok := f.tt.Find(weightedProgressLabel)
	if !ok || bar.H <= 0 || bar.Y < strip.Y+strip.H {
		t.Fatal("progress inaccessible after scroll", bar, strip)
	}
	for _, frac := range []float32{0.01, 0.5, 0.99} {
		// Empty track, coloured fill and percentage glyph all belong to row input.
		f.tt.ClickAt(bar.X+bar.W*frac, bar.Y+bar.H/2)
		f.all()
		if f.s.runtimeSelection != string(tr.ID) || !f.s.runtimeDayRail {
			t.Fatal("readonly overlay swallowed click", frac)
		}
		f.s.titleTransition(func() { f.s.runtimeSelection = ""; f.s.runtimeDayRail = false })
		f.all()
		bar, _ = f.tt.Find(weightedProgressLabel)
	}
	if before != f.h.progress.projection {
		t.Fatal("scroll/click rebuilt projection")
	}
	progressSavePNG(t, f.tt, "review-scrolled-list-overlay-2x.png")
}

func TestReviewProgressUIFutureHistoryAndNoopCommand(t *testing.T) {
	f, tr, _, hard := progressFixtureUI(t)
	before := f.h.progress.projection
	f.h.command(f.s, func() error { return f.svc.SetSubtaskDifficulty(tr.ChainID, tr.ID, hard.ID, core.DifficultyHard) }, false, nil)
	f.all()
	if before != f.h.progress.projection {
		t.Fatal("successful no-op command rebuilt projection")
	}
	f.s.returnTaskToPool(tr.ChainID, tr.ID)
	f.all()
	tomorrow, _ := liveTestDay.AddDays(1)
	f.h.command(f.s, func() error { return f.svc.Schedule(tr.ChainID, tomorrow) }, false, nil)
	f.all()
	g := f.svc.Snapshot()
	future := g.Traces[len(g.Traces)-1]
	f.s.titleTransition(func() {
		f.s.runtimeDate = tomorrow
		f.s.runtimeSelection = string(future.ID)
		f.s.runtimeDayRail = true
	})
	f.all()
	wantUIProgress(t, f.s, future.ID, 25, 1, 4)
	progressAccess(t, f.tt, core.ProgressWeightedSubtasks, 25)
	before = f.h.progress.projection
	liveClick(t, f.tt, "切换完成 "+string(future.ID))
	f.all()
	if !reflect.DeepEqual(g, f.svc.Snapshot()) || before != f.h.progress.projection {
		t.Fatal("future completion click changed read-only facts")
	}
	f.s.titleTransition(func() { f.s.runtimeDayRail = false; f.s.runtimeSelection = "" })
	f.all()
	bar, ok := f.tt.Find(weightedProgressLabel)
	if !ok {
		t.Fatal("future history-backed progress not displayed")
	}
	f.tt.RightClickAt(bar.X+bar.W-1, bar.Y+bar.H/2)
	f.all()
	if !reflect.DeepEqual(f.tt.Menu(), []string{returnToPoolLabel}) {
		t.Fatal("progress overlay blocked or changed native future menu", f.tt.Menu())
	}
	f.tt.CloseMenu()
	f.all()
	if !reflect.DeepEqual(g, f.svc.Snapshot()) {
		t.Fatal("cancelled menu changed facts")
	}
	f.tt.SetScale(2)
	progressSavePNG(t, f.tt, "review-future-history-2x.png")
}
