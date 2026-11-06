package taskapp

import (
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func ricRejected(t *testing.T, q historyCommandProof) {
	t.Helper()
	r := auditedHistoryCommand{&q}
	if r.valid() {
		t.Fatal("invalid detail combination accepted")
	}
	if _, e := r.snapshotCopy(); e == nil {
		t.Fatal("snapshotCopy escaped valid")
	}
	if _, e := r.changed(); e == nil {
		t.Fatal("changed escaped valid")
	}
}
func TestRemainingRevisionClosureMatrix(t *testing.T) {
	var fresh []auditedHistoryCommand
	{
		f := vnSeed(t, vnDate)
		b := f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		in := riReturn(f, vnAt(5))
		r, e := auditReturnHistoryVNext(p, b, riReplayR(t, b, in), in)
		must(t, e)
		fresh = append(fresh, r)
	}
	{
		f := vnSeed(t, vnDate)
		b := f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		i := riCarry(f, vnAt(5))
		must(t, f.k.DeferCurrentTrace(i))
		r, e := auditDeferHistoryVNext(p, b, f.k.Snapshot(), riD(i))
		must(t, e)
		fresh = append(fresh, r)
	}
	{
		f := vnSeed(t, vnDate)
		f.run(t, vnStep{"L", 2})
		b := f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		i := riCarry(f, vnAt(5))
		i.Today = i.TargetDate
		must(t, f.k.ContinueUnfinishedTrace(i))
		r, e := auditContinueHistoryVNext(p, b, f.k.Snapshot(), riC(i))
		must(t, e)
		fresh = append(fresh, r)
	}
	for ni, r := range fresh {
		t.Run(fmt.Sprintf("new-%d", ni), func(t *testing.T) {
			if !r.valid() {
				t.Fatal("real new proof invalid")
			}
			for kind := historyCommandSettlement; kind <= historyCommandActualSubtaskEdit; kind++ {
				q := *r.proof
				q.kind = kind
				ricRejected(t, q)
			}
			for _, kind := range []historyCommandKind{historyCommandReturn, historyCommandDefer, historyCommandContinue} {
				for mask := 0; mask < 8; mask++ {
					q := *r.proof
					q.kind = kind
					q.returnDetail = nil
					q.deferDetail = nil
					q.continueDetail = nil
					if mask&1 != 0 {
						q.returnDetail = &returnHistoryInput{}
					}
					if mask&2 != 0 {
						q.deferDetail = &deferHistoryInput{}
					}
					if mask&4 != 0 {
						q.continueDetail = &continueHistoryInput{}
					}
					want := (kind == historyCommandReturn && mask == 1) || (kind == historyCommandDefer && mask == 2) || (kind == historyCommandContinue && mask == 4)
					if want {
						if !(auditedHistoryCommand{&q}).valid() {
							t.Fatal("single matching detail invalid")
						}
					} else {
						ricRejected(t, q)
					}
				}
			}
			for which := 0; which < 5; which++ {
				q := *r.proof
				switch which {
				case 0:
					q.seal = nil
				case 1:
					q.prior = validatedHistorySnapshot{}
				case 2:
					q.after = validatedHistorySnapshot{}
				case 3:
					q.kind = 0
				case 4:
					q.kind = 255
				}
				ricRejected(t, q)
			}
		})
	}
	// Generate actual old completion and undo proofs, not hand-made validity shells.
	f := vnSeed(t, vnDate)
	for _, action := range []completionHistoryAction{completionHistoryComplete, completionHistoryUndo} {
		b := f.k.Snapshot()
		p := vnGood(t, f.snapshot(t))
		now := vnAt(5)
		if action == completionHistoryComplete {
			must(t, f.k.MarkCompleted(core.DayTraceID(vnID(3)), vnDate, now))
		} else {
			now = vnAt(6)
			must(t, f.k.UndoCompleted(core.DayTraceID(vnID(3)), vnDate, now))
		}
		r, e := auditCompletionHistoryVNext(p, b, f.k.Snapshot(), completionHistoryInput{Action: action, ChainID: core.TaskChainID(vnID(1)), TraceID: core.DayTraceID(vnID(3)), Today: vnDate, Now: now})
		must(t, e)
		if !r.valid() || r.proof.returnDetail != nil || r.proof.deferDetail != nil || r.proof.continueDetail != nil {
			t.Fatal("actual old proof compatibility")
		}
		if _, e := r.snapshotCopy(); e != nil {
			t.Fatal(e)
		}
		if _, e := r.changed(); e != nil {
			t.Fatal(e)
		}
		for kind := historyCommandSettlement; kind <= historyCommandActualSubtaskEdit; kind++ {
			q := *r.proof
			q.kind = kind
			if !(auditedHistoryCommand{&q}).valid() {
				t.Fatal("old kind nil-detail closed branch changed")
			}
			for detail := 0; detail < 3; detail++ {
				dirty := q
				switch detail {
				case 0:
					dirty.returnDetail = fresh[0].proof.returnDetail
				case 1:
					dirty.deferDetail = fresh[1].proof.deferDetail
				case 2:
					dirty.continueDetail = fresh[2].proof.continueDetail
				}
				ricRejected(t, dirty)
			}
		}
	}
}
