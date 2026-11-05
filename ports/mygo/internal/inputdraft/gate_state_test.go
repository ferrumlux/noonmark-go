// SPDX-License-Identifier: AGPL-3.0-only
package inputdraft

import "testing"

// 对事件序列作有界穷举，并用独立的前态规则校验每次转换。
// 这不是 Swift 动态 oracle，也不声称穷举 uint64 状态空间。
func TestBoundedEventSequencesMatchSourceTransitions(t *testing.T) {
	const depth, alphabet = 5, 12
	var transitions uint64
	var walk func(Gate, *Schedule, *PersistenceRequest, int)
	walk = func(prior Gate, latestSchedule *Schedule, latestRequest *PersistenceRequest, remaining int) {
		if remaining == 0 {
			return
		}
		for event := 0; event < alphabet; event++ {
			g, expected := prior, prior
			var output *Schedule
			var started *PersistenceRequest
			matching := latestRequest != nil && prior.hasInFlight && *latestRequest == prior.inFlight && latestRequest.OwnerID == prior.ownerID && latestRequest.Generation == prior.generation
			eligible := latestSchedule != nil && latestSchedule.OwnerID == prior.ownerID && latestSchedule.Generation == prior.generation && latestSchedule.Revision == prior.revision && latestSchedule.Sequence == prior.scheduleSequence && !prior.isComposing && prior.revision != prior.acknowledgedRevision && !prior.hasInFlight
			shouldSchedule := false
			delay := uint64(700)
			if prior.explicitFlushPending {
				delay = 0
			}
			switch event {
			case 0, 1, 2, 3, 4, 5:
				textChanged := event == 0 || event == 3 || event == 4
				nextComposing := prior.isComposing
				if event == 1 || event == 3 {
					nextComposing = true
				}
				if event == 2 || event == 4 {
					nextComposing = false
				}
				changed := textChanged || nextComposing != prior.isComposing
				if changed {
					if textChanged {
						expected.revision++
						expected.retryAttempt = 0
					}
					expected.isComposing = nextComposing
					expected.scheduleSequence++
					shouldSchedule = !nextComposing && expected.revision != expected.acknowledgedRevision
				}
				switch event {
				case 0:
					output = g.DraftDidChange()
				case 1:
					output = g.CompositionDidChange(true)
				case 2:
					output = g.CompositionDidChange(false)
				case 3:
					output = g.NativeSnapshotDidChange(true, true)
				case 4:
					output = g.NativeSnapshotDidChange(true, false)
				case 5:
					output = g.NativeSnapshotDidChange(false, prior.isComposing)
				}
			case 6:
				expected.explicitFlushPending = true
				shouldSchedule = !prior.isComposing && prior.revision != prior.acknowledgedRevision && !prior.hasInFlight
				delay = 0
				output = g.RequestFlush()
			case 7:
				if latestSchedule != nil {
					started = g.BeginPersistence(*latestSchedule)
				}
				if eligible {
					expected.requestSequence++
					expected.hasInFlight = true
					expected.inFlight = PersistenceRequest{OwnerID: prior.ownerID, Generation: prior.generation, Revision: prior.revision, RequestID: expected.requestSequence}
					if started == nil || *started != expected.inFlight {
						t.Fatalf("begin request mismatch at depth %d", remaining)
					}
				} else if started != nil {
					t.Fatal("ineligible begin returned request")
				}
			case 8:
				if latestRequest != nil {
					output = g.PersistenceDidSucceed(*latestRequest)
				}
				if matching {
					expected.hasInFlight = false
					expected.inFlight = PersistenceRequest{}
					if latestRequest.Revision > expected.acknowledgedRevision {
						expected.acknowledgedRevision = latestRequest.Revision
					}
					expected.retryAttempt = 0
					shouldSchedule = expected.revision != expected.acknowledgedRevision
					if !shouldSchedule {
						expected.explicitFlushPending = false
						expected.scheduleSequence++
					}
				}
			case 9:
				if latestRequest != nil {
					output = g.PersistenceDidFail(*latestRequest)
				}
				if matching {
					expected.hasInFlight = false
					expected.inFlight = PersistenceRequest{}
					expected.retryAttempt++
					shouldSchedule = !prior.isComposing && prior.revision != prior.acknowledgedRevision
					// 深度 5 内最多两个失败；表格仍定义完整 1/2/4/8/10 秒序列。
					delays := [...]uint64{1000, 2000, 4000, 8000, 10000}
					index := expected.retryAttempt - 1
					if index > 4 {
						index = 4
					}
					delay = delays[index]
				}
			case 10:
				expected.acknowledgedRevision = prior.revision
				expected.explicitFlushPending = false
				expected.retryAttempt = 0
				expected.hasInFlight = false
				expected.inFlight = PersistenceRequest{}
				expected.scheduleSequence++
				g.AcknowledgeWithoutPersistence()
			case 11:
				expected.generation++
				expected.revision = 0
				expected.acknowledgedRevision = 0
				expected.explicitFlushPending = false
				expected.retryAttempt = 0
				expected.isComposing = false
				expected.hasInFlight = false
				expected.inFlight = PersistenceRequest{}
				expected.scheduleSequence++
				g.DiscardLocalChanges()
			}
			if shouldSchedule {
				expected.scheduleSequence++
				want := Schedule{OwnerID: expected.ownerID, Generation: expected.generation, Revision: expected.revision, Sequence: expected.scheduleSequence, DelayMilliseconds: delay}
				if output == nil || *output != want {
					t.Fatalf("event %d remaining %d schedule=%+v want=%+v", event, remaining, output, want)
				}
			} else if output != nil {
				t.Fatalf("event %d unexpectedly scheduled", event)
			}
			if g != expected {
				t.Fatalf("event %d remaining %d\ngot=%+v\nwant=%+v", event, remaining, g, expected)
			}
			if g.OwnerID() != g.ownerID || g.Generation() != g.generation || g.IsComposing() != g.isComposing || g.HasUnflushedChanges() != (g.revision != g.acknowledgedRevision) || g.HasPersistenceInFlight() != g.hasInFlight {
				t.Fatal("state query mismatch")
			}
			if output != nil {
				latest := *output
				output = &latest
			} else {
				output = latestSchedule
			}
			if started == nil {
				started = latestRequest
			}
			wantPermit := output != nil && output.OwnerID == g.ownerID && output.Generation == g.generation && output.Revision == g.revision && output.Sequence == g.scheduleSequence && !g.isComposing && g.revision != g.acknowledgedRevision && !g.hasInFlight
			if g.PermitsAutosave(output) != wantPermit {
				t.Fatal("source predicate mismatch")
			}
			transitions++
			walk(g, output, started, remaining-1)
		}
	}
	walk(*mustGate(t), nil, nil, depth)
	if transitions != 271452 {
		t.Fatalf("unexpected exploration size: %d", transitions)
	}
	t.Logf("checked %d transitions, all 248832 length-5 sequences over 12 events", transitions)
}
