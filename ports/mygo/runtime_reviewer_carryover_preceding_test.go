package main

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestReviewerEarlierCommandCannotRemoveGuardedNativeOwner(t *testing.T) {
	for _, action := range []string{"标记完成", returnToPoolLabel} {
		t.Run(action, func(t *testing.T) {
			f := newTitleFixture(t, true)
			tr := f.svc.Snapshot().Traces[0]
			prior := newRuntimeApp(f.h.readView())
			f.h.attach(prior, func() {}, func() {})
			pt := ui.NewTester(prior.view, 1200, 768)
			owner := newRuntimeApp(f.h.readView())
			f.h.attach(owner, func() {}, func() {})
			owner.runtimeSelection = string(tr.ID)
			owner.runtimeDayRail = true
			ot := ui.NewTester(owner.view, 1200, 768)
			f.ui()
			ot.Frame()
			openCarryoverMenu(t, f, carryoverDefer)
			openReturnMenu(t, pt, tr.ID)
			if err := pt.ChooseMenuItem(action); err != nil {
				t.Fatal(err)
			}
			f.ui()
			confirmCarryover(t, f)
			if len(f.io.q) != 2 {
				t.Fatal("expected earlier mutation and later Defer", len(f.io.q))
			}
			label := subtaskPrefix("day", string(tr.ID)) + ".new"
			liveClick(t, ot, label)
			var original *runtimeSubtaskComposer
			for _, c := range owner.subtaskComposers {
				if c.target.trace == tr.ID {
					original = c
				}
			}
			titleDirectClient(t, ot).SetMarkedText(nil, "先行命令期间送达", ui.TextInputRange{Start: 8, End: 8})
			f.io.one()
			f.ui()
			ot.Frame()
			snap, ok := original.native.handle.TextInputSnapshot(original.native.context)
			if !ok || !strings.Contains(snap.Text+snap.MarkedText, "先行命令期间送达") || !original.mounted {
				t.Fatal("earlier source mutation removed guarded native owner before carryover completion", snap, ok)
			}
			f.all()
			ot.Frame()
			f.ui()
			if strings.Contains(owner.runtimeError, "延期已保存") {
				t.Fatal("other mutation incorrectly represented as defer success")
			}
			if len(f.svc.Snapshot().Traces) != 1 {
				t.Fatal("rejected defer unexpectedly committed")
			}
		})
	}
}
