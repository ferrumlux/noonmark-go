// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func manualAuditFixture(t *testing.T) (*Service, core.TaskGraph, core.TaskGraph, core.SetManualProgressInput) {
	t.Helper()
	s, _, chain, trace := manualServiceFixture(t)
	must(t, s.AddToday("合成独立任务"))
	before := s.Snapshot()
	now, err := s.nextInstant()
	must(t, err)
	command := core.SetManualProgressInput{ChainID: chain, TraceID: trace, Percent: 37, Today: testDay, Now: now}
	k, err := core.NewTaskKernel(before)
	must(t, err)
	must(t, k.SetManualProgress(command))
	return s, before, k.Snapshot(), command
}

func TestManualProgressReplayAuditRejectsUnrequestedFacts(t *testing.T) {
	s, before, after, command := manualAuditFixture(t)
	priorBytes, err := canonical.Marshal(s.snapshot)
	must(t, err)
	accepted, err := captureManualProgressHistory(s.snapshot, before, after, command)
	must(t, err)
	_, decoded, err := normalizeSnapshot(accepted)
	must(t, err)
	if !reflect.DeepEqual(after, decoded) {
		t.Fatal("canonical round trip lost manual value or clock")
	}
	for _, kind := range []string{"percent", "clock", "status", "priority", "description", "other-trace", "chain", "definition", "day", "remove-trace"} {
		t.Run(kind, func(t *testing.T) {
			forged := after.Clone()
			switch kind {
			case "percent":
				*forged.Traces[0].ManualProgressPercent = 36
			case "clock":
				forged.Traces[0].ContentUpdatedAt = before.Traces[0].ContentUpdatedAt
			case "status":
				forged.Traces[0].Status = core.TraceCompleted
				forged.Traces[0].CompletedAt = &command.Now
			case "priority":
				forged.Traces[0].Priority += 10
			case "description":
				text := "伪造说明"
				forged.Traces[0].DescriptionText = &text
			case "other-trace":
				value := 1
				forged.Traces[1].ManualProgressPercent = &value
			case "chain":
				forged.Chains[0].UpdatedAt = command.Now
			case "definition":
				forged.Definitions[0].Title = "伪造标题"
			case "day":
				text := "伪造复盘"
				forged.Days[0].ReviewSummary = &text
			case "remove-trace":
				forged.Traces = forged.Traces[:1]
			}
			if _, err := captureManualProgressHistory(s.snapshot, before, forged, command); err == nil {
				t.Fatal("replay accepted unrelated mutation")
			}
		})
	}
	for _, kind := range []string{"chain", "trace", "percent", "clock", "today"} {
		t.Run("wrong-command/"+kind, func(t *testing.T) {
			wrong := command
			switch kind {
			case "chain":
				wrong.ChainID = before.Chains[1].ID
			case "trace":
				wrong.TraceID = before.Traces[1].ID
				wrong.ChainID = before.Chains[1].ID
			case "percent":
				wrong.Percent++
			case "clock":
				wrong.Now = before.Traces[0].ContentUpdatedAt
			case "today":
				wrong.Today, _ = testDay.AddDays(1)
			}
			if _, err := captureManualProgressHistory(s.snapshot, before, after, wrong); err == nil {
				t.Fatal("wrong command accepted")
			}
		})
	}
	badBefore := before.Clone()
	badBefore.Definitions[0].Title = "伪造起点"
	if _, err := captureManualProgressHistory(s.snapshot, badBefore, after, command); err == nil {
		t.Fatal("mismatched prior accepted")
	}
	actualPrior, err := canonical.Marshal(s.snapshot)
	must(t, err)
	if string(priorBytes) != string(actualPrior) {
		t.Fatal("audit mutated prior")
	}
}

func TestManualProgressAuditKeepsEmptyCatalogAndMissingHistoryFailClosed(t *testing.T) {
	s, before, after, command := manualAuditFixture(t)
	wire, err := canonical.Marshal(s.snapshot)
	must(t, err)
	for _, field := range []string{"categories", "labels", "currentByChainID", "relationHistory", "changeRecords", "committedReceiptsByInteractionID"} {
		t.Run(field, func(t *testing.T) {
			bad, err := canonical.Parse(wire, canonical.DefaultLimits())
			must(t, err)
			bad.Object["classifications"].Object[field] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{historyObject(map[string]canonical.Value{})}}
			if _, err = captureManualProgressHistory(bad, before, after, command); err == nil {
				t.Fatal("unsupported catalog accepted")
			}
		})
	}
	// Generate real settlement history, remove it, and verify it cannot be
	// reconstructed by a manual command on an unrelated current trace.
	tomorrow, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(tomorrow))
	must(t, s.AddToday("跨日合成任务"))
	before = s.Snapshot()
	var tr core.DayTrace
	for _, candidate := range before.Traces {
		if candidate.Status == core.TracePending {
			tr = candidate
		}
	}
	now, err := s.nextInstant()
	must(t, err)
	command = core.SetManualProgressInput{ChainID: tr.ChainID, TraceID: tr.ID, Percent: 50, Today: tomorrow, Now: now}
	k, err := core.NewTaskKernel(before)
	must(t, err)
	must(t, k.SetManualProgress(command))
	after = k.Snapshot()
	valid, err := captureManualProgressHistory(s.snapshot, before, after, command)
	must(t, err)
	if !historyEqual(valid.Object["classifications"], s.snapshot.Object["classifications"]) {
		t.Fatal("audit rewritten")
	}
	_, extra, err := v7.ToDomain(s.snapshot)
	must(t, err)
	neutral, err := emptySupplement()
	must(t, err)
	extra.Classifications = neutral.Classifications
	missing, err := v7.FromDomain(recurrence.State{Tasks: before}, extra)
	must(t, err)
	if _, err = captureManualProgressHistory(missing, before, after, command); err == nil {
		t.Fatal("missing history reconstructed")
	}
}

func TestManualProgressExactClockBitsAndOptionalCodec(t *testing.T) {
	s, _, chain, trace := manualServiceFixture(t)
	initial, err := encodeSnapshot(s.snapshot)
	must(t, err)
	wire, err := decodeSnapshot(initial)
	must(t, err)
	state, _, err := v7.ToDomain(wire)
	must(t, err)
	if state.Tasks.Traces[0].ManualProgressPercent != nil {
		t.Fatal("codec lost nil")
	}
	must(t, s.SetManualProgress(chain, trace, 0))
	before := s.Snapshot()
	encoded, err := encodeSnapshot(s.snapshot)
	must(t, err)
	wire, err = decodeSnapshot(encoded)
	must(t, err)
	state, _, err = v7.ToDomain(wire)
	must(t, err)
	if state.Tasks.Traces[0].ManualProgressPercent == nil || *state.Tasks.Traces[0].ManualProgressPercent != 0 || uint64(state.Tasks.Traces[0].ContentUpdatedAt) != uint64(before.Traces[0].ContentUpdatedAt) {
		t.Fatal("codec changed zero/clock bits")
	}
	for _, invalid := range []int{-1, 101} {
		bad := before.Clone()
		*bad.Traces[0].ManualProgressPercent = invalid
		if err := core.ValidateTaskGraph(bad); err == nil {
			t.Fatal("manual range validator relaxed")
		}
		_, extra, err := v7.ToDomain(s.snapshot)
		must(t, err)
		if _, err = v7.FromDomain(recurrence.State{Tasks: bad}, extra); err == nil {
			t.Fatal("codec accepted invalid range")
		}
	}
}
