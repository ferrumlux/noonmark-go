// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Preserve the real read view and service. Only count calls; never supply a
// canned graph or collapse the Tester's separate view builds into one frame.
type daySnapshotCounter struct {
	taskService
	snapshots int
	last      core.TaskGraph
}

func (c *daySnapshotCounter) Snapshot() core.TaskGraph {
	c.snapshots++
	c.last = c.taskService.Snapshot()
	return c.last
}

func TestDaySnapshotOneReadPerViewBuild(t *testing.T) {
	for _, count := range []int{2, 34} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := newTitleFixture(t, true)
			for i := 1; i < count; i++ {
				if err := f.svc.AddToday(fmt.Sprintf("快照计数任务%d", i)); err != nil {
					t.Fatal(err)
				}
			}
			f.h.publish(f.svc.Snapshot(), f.svc.Today())
			counter := &daySnapshotCounter{taskService: f.s.runtime}
			f.s.runtime = counter
			builds := 0
			var reads []int
			f.tt = ui.NewTester(func(c *ui.Context) {
				before := counter.snapshots
				f.s.view(c)
				builds++
				reads = append(reads, counter.snapshots-before)
			}, 1200, 768)
			f.tt.Frame()
			if builds == 0 || len(reads) != builds {
				t.Fatal("real native view did not run")
			}
			for n, got := range reads {
				if got != 1 {
					t.Errorf("view build %d with %d rows read %d whole graphs, want exactly its own single snapshot", n, count, got)
				}
			}
			t.Logf("real Tester builds=%d rows=%d snapshot_calls=%d", builds, count, counter.snapshots)
		})
	}
}

// Every native view build gets a new defensive read, even when publication
// occurs between two builds performed by the same live Tester.
func TestDaySnapshotRevisionsAndIsolation(t *testing.T) {
	f := newTitleFixture(t, true)
	counter := &daySnapshotCounter{taskService: f.s.runtime}
	f.s.runtime = counter
	var reads []core.TaskGraph
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.s.view(c)
		reads = append(reads, counter.last)
	}, 1200, 768)
	before := reads[len(reads)-1]
	preserved := before.Clone()
	_, revision := yearSnapshot(t, f.path)
	for _, title := range []string{"本次新版本一", "本次新版本二"} {
		if err := f.svc.AddToday(title); err != nil {
			t.Fatal(err)
		}
		f.h.publish(f.svc.Snapshot(), f.svc.Today())
		f.tt.Frame()
		_, next := yearSnapshot(t, f.path)
		if next != revision+1 {
			t.Fatal("real command did not create the next durable revision", revision, next)
		}
		revision = next
		if !f.tt.HasText(title) {
			t.Fatal("next native view build reused an old graph", title)
		}
		if !reflect.DeepEqual(before, preserved) {
			t.Fatal("later publication mutated a prior defensive snapshot")
		}
	}
	current := f.h.graph.Clone()
	copy := f.s.runtime.Snapshot()
	copy.Definitions[0].Title = "不可污染发布图"
	copy.Traces[0].Status = core.TraceAbandoned
	if !reflect.DeepEqual(current, f.h.graph) || !reflect.DeepEqual(current, f.svc.Snapshot()) {
		t.Fatal("defensive read exposed shared mutable storage")
	}
}

// The graph can be shared within one view build, but editor overlays cannot:
// derive them afresh after real native edits, a failed write and publication.
func TestDaySnapshotOverlayRecomputed(t *testing.T) {
	f, trace, child := subtaskFixture(t)
	liveClick(t, f.tt, "展开子任务 "+string(trace.ID))
	f.all()
	parent := runtimeSubtaskTarget{chain: trace.ChainID, trace: trace.ID}
	graph := f.s.runtime.Snapshot()
	preserved := graph.Clone()
	assertRows := func() {
		t.Helper()
		got := f.s.childRowsFromGraph(graph, liveTestDay, "day-list", parent)
		fresh := f.s.childRows("day-list", parent)
		if !reflect.DeepEqual(got, fresh) {
			t.Fatal("same facts lost current editor overlay")
		}
		if !reflect.DeepEqual(graph, preserved) {
			t.Fatal("overlay calculation mutated its private graph")
		}
	}
	assertRows()
	label := subtaskPrefix("day-list", string(child.ID)) + ".title"
	subtaskEdit(t, f, label, "本次构建的未保存草稿")
	assertRows()
	titleStoreFault(f.svc, func(phase string) error {
		if phase == "before-reserve" {
			return os.ErrPermission
		}
		return nil
	})
	f.advance(700 * time.Millisecond)
	f.all()
	e := f.s.subtaskEditors[subtaskOwner("day-list", string(child.ID))]
	if e == nil || e.failure == nil || e.native.draft != "本次构建的未保存草稿" {
		t.Fatal("real failed editor fixture missing")
	}
	assertRows()
	titleStoreFault(f.svc, nil)
	if err := f.svc.DeleteSubtask(trace.ChainID, trace.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	f.all()
	graph = f.s.runtime.Snapshot()
	preserved = graph.Clone()
	assertRows()
	got := f.s.childRowsFromGraph(graph, liveTestDay, "day-list", parent)
	if len(got) != 1 || !got[0].retained || got[0].editable || got[0].toggle {
		t.Fatal("deleted dirty owner was not retained read-only", got)
	}
	if e.native.draft != "本次构建的未保存草稿" {
		t.Fatal("publication destroyed native draft")
	}
}

// A callback posted by the old native surface must not gain write authority
// from that build's otherwise valid private graph after a newer real revision.
func TestDaySnapshotQueuedComposerUsesNewFacts(t *testing.T) {
	f := newTitleFixture(t, true)
	trace := f.svc.Snapshot().Traces[0]
	parent := runtimeSubtaskTarget{chain: trace.ChainID, trace: trace.ID}
	old := f.s.runtime.Snapshot()
	if !f.s.canAddSubtaskFromGraph(old, liveTestDay, parent) {
		t.Fatal("initial parent is not writable")
	}
	label := subtaskPrefix("day", string(trace.ID)) + ".new"
	subtaskEdit(t, f, label, "旧界面的排队新增")
	f.tt.Key(0, ui.KeyEnter)
	if len(f.posts.q) == 0 {
		t.Fatal("native submit did not post its actual callback")
	}
	if err := f.svc.SetCompleted(trace.ID, true); err != nil {
		t.Fatal(err)
	}
	f.h.publish(f.svc.Snapshot(), liveTestDay)
	_, committed := yearSnapshot(t, f.path)
	if f.s.canAddSubtask(parent) {
		t.Fatal("interaction wrapper reused the old drawable graph")
	}
	if !f.s.canAddSubtaskFromGraph(old, liveTestDay, parent) {
		t.Fatal("old defensive snapshot unexpectedly changed")
	}
	f.all()
	_, after := yearSnapshot(t, f.path)
	if after != committed || len(f.svc.Snapshot().Subtasks) != 0 {
		t.Fatal("stale queued native event wrote against a new completed parent")
	}
	retained := false
	for _, c := range f.s.subtaskComposers {
		if c.target == parent && c.native.draft == "旧界面的排队新增" && c.failure != nil {
			retained = true
		}
	}
	if !retained {
		t.Fatal("failed stale submission lost its original recovery draft")
	}
}

func TestDaySnapshotExpandedChildrenOneReadPerViewBuild(t *testing.T) {
	for _, count := range []int{2, 34} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f, trace, child := subtaskFixture(t)
			for i := 1; i < count; i++ {
				if err := f.svc.AddToday(fmt.Sprintf("展开快照任务%d", i)); err != nil {
					t.Fatal(err)
				}
			}
			f.h.publish(f.svc.Snapshot(), f.svc.Today())
			f.all()
			liveClick(t, f.tt, "展开子任务 "+string(trace.ID))
			f.all()
			counter := &daySnapshotCounter{taskService: f.s.runtime}
			f.s.runtime = counter
			var reads []int
			f.tt = ui.NewTester(func(c *ui.Context) {
				before := counter.snapshots
				f.s.view(c)
				reads = append(reads, counter.snapshots-before)
			}, 1200, 768)
			f.tt.Frame()
			for _, surface := range []string{"day-list", "day"} {
				if _, ok := f.tt.Find(subtaskPrefix(surface, string(child.ID)) + ".title"); !ok {
					t.Fatal("real expanded list/detail child missing", surface)
				}
			}
			if len(reads) < 2 {
				t.Fatal("Tester did not perform independent native view builds")
			}
			for n, got := range reads {
				if got != 1 {
					t.Errorf("expanded view build %d with %d parents read %d graphs", n, count, got)
				}
			}
			t.Logf("real expanded list+detail Tester builds=%d parents=%d snapshot_calls=%d", len(reads), count, counter.snapshots)
		})
	}
}
