// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func TestHistoryEditTitleReuse(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	in := titleHistoryInput{Action: titleHistorySaveInput, Route: titleHistoryChain, ChainID: before.Chains[0].ID, Title: "changed", Today: vnDate, Now: vnAt(2)}
	must(t, f.k.SaveTaskTitleInput(core.EditTaskTitleInput{ChainID: in.ChainID, Title: in.Title, Today: in.Today, Now: in.Now}))
	r, e := auditTitleHistoryVNext(prior, before, f.k.Snapshot(), in)
	must(t, e)
	got, e := r.snapshotCopy()
	must(t, e)
	if !r.valid() || !historyEqual(got, f.snapshot(t)) || len(f.k.Snapshot().Definitions) != len(before.Definitions) {
		t.Fatal("reuse proof")
	}
}
func TestHistoryEditManual(t *testing.T) {
	f := vnSeed(t, vnDate)
	before := f.k.Snapshot()
	prior := vnGood(t, f.snapshot(t))
	in := manualProgressHistoryInput{ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Percent: 100, Today: vnDate, Now: vnAt(2)}
	must(t, f.k.SetManualProgress(core.SetManualProgressInput(in)))
	r, e := auditManualProgressHistoryVNext(prior, before, f.k.Snapshot(), in)
	must(t, e)
	if !r.valid() || f.k.Snapshot().Traces[0].Status != core.TracePending {
		t.Fatal("manual proof")
	}
}
