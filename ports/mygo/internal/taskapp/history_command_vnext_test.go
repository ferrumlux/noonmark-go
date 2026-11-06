// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"testing"
)

func TestHistoryCommandSettlementSources(t *testing.T) {
	cases := map[string][]vnStep{
		"P": {}, "C": {{"C", 2}}, "A": {{"A", 2}}, "U": {{"A", 2}, {"U", 3}},
		"R": {{"R", 2}}, "D": {{"D", 2}}, "X": {{"X", 2}},
		"old A completed": {{"A", 1}, {"P", 2}, {"C", 3}},
		"locked U":        {{"L", 3}}, "locked C": {{"C", 2}, {"L", 3}},
		"locked A": {{"A", 2}, {"L", 3}}, "locked R": {{"R", 2}, {"L", 3}}, "locked D": {{"D", 2}, {"L", 3}},
	}
	for name, steps := range cases {
		t.Run(name, func(t *testing.T) {
			f := vnSeed(t, vnDate)
			f.run(t, steps...)
			before := f.k.Snapshot()
			prior := vnGood(t, f.snapshot(t))
			count := f.captures
			f.run(t, vnStep{"L", 5})
			after := f.k.Snapshot()
			want := f.snapshot(t)
			var ids []string
			for i := count + 1; i <= f.captures; i++ {
				ids = append(ids, vnID(100+i))
			}
			today, e := vnDate.AddDays(1)
			must(t, e)
			result, e := auditSettlementHistoryVNext(prior, before, after, settlementHistoryInput{Today: today, Now: vnAt(5), EventIDs: ids})
			must(t, e)
			got, e := result.snapshotCopy()
			must(t, e)
			if !historyEqual(got, want) {
				t.Fatal("not exact source projection")
			}
			changed, e := result.changed()
			must(t, e)
			if changed == historyEqual(prior.proof.snapshot, want) {
				t.Fatal("incorrect changed")
			}
		})
	}
}

func TestHistoryCommandCompletionSourcesAndMaliciousDelta(t *testing.T) {
	f := vnSeed(t, vnDate)
	f.run(t, vnStep{"A", 1}, vnStep{"P", 2}, vnStep{"A", 3}, vnStep{"P", 4})
	for _, step := range []vnStep{{"C", 5}, {"undo", 6}, {"C", 7}, {"undo", 8}} {
		before := f.k.Snapshot()
		prior := vnGood(t, f.snapshot(t))
		f.run(t, step)
		after := f.k.Snapshot()
		action := completionHistoryComplete
		if step.op == "undo" {
			action = completionHistoryUndo
		}
		in := completionHistoryInput{Action: action, ChainID: before.Chains[0].ID, TraceID: before.Traces[0].ID, Today: vnDate, Now: vnAt(step.at)}
		result, e := auditCompletionHistoryVNext(prior, before, after, in)
		must(t, e)
		got, e := result.snapshotCopy()
		must(t, e)
		if !historyEqual(got, f.snapshot(t)) || !historyEqual(got.Object["classifications"], prior.proof.snapshot.Object["classifications"]) {
			t.Fatal("complete/undo lost old A")
		}
		bad := after.Clone()
		bad.Definitions[0].Title = "unrelated legal mutation"
		if _, e = auditCompletionHistoryVNext(prior, before, bad, in); e == nil {
			t.Fatal("malicious after accepted")
		}
		bad = before.Clone()
		bad.Definitions[0].Title = "unrelated before"
		if _, e = auditCompletionHistoryVNext(prior, bad, after, in); e == nil {
			t.Fatal("unbound before accepted")
		}
	}
}
