// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAggregateRolloverCapturesAndPreservesEmptyHistory(t *testing.T) {
	s, p := fresh(t)
	must(t, s.AddToday("pending"))
	must(t, s.AddToday("complete"))
	must(t, s.SetCompleted(s.Snapshot().Traces[1].ID, true))
	beforeRevision := s.disk.revision
	next, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(next))
	if s.disk.revision != beforeRevision+1 || s.Today() != next {
		t.Fatal("rollover was not one atomic revision")
	}
	classes := s.snapshot.Object["classifications"]
	if historyUint(classes.Object["revision"]) != 2 || len(classes.Object["snapshotEventsByTraceID"].Array) != 4 {
		t.Fatal("empty classification captures omitted")
	}
	for i := 1; i < 4; i += 2 {
		event := classes.Object["snapshotEventsByTraceID"].Array[i].Array[0]
		if historyUint(event.Object["capturedAt"]) != uint64(*s.graph.Days[0].LockedAt) {
			t.Fatal("capture and lock clock differ")
		}
	}
	original, e := canonical.Marshal(classes)
	must(t, e)
	must(t, s.AddToday("next day"))
	after, e := canonical.Marshal(s.snapshot.Object["classifications"])
	must(t, e)
	if string(original) != string(after) {
		t.Fatal("current-day command changed prior audit")
	}
	saved, e := encodeSnapshot(s.snapshot)
	must(t, e)
	revision := s.disk.revision
	s.Close()
	r, e := Open(p, next)
	must(t, e)
	defer r.Close()
	restored, e := encodeSnapshot(r.snapshot)
	must(t, e)
	if string(saved) != string(restored) || r.disk.revision != revision {
		t.Fatal("reopen changed event IDs or duplicated capture")
	}
	third, _ := next.AddDays(1)
	must(t, r.AdvanceTo(third))
	if historyUint(r.snapshot.Object["classifications"].Object["revision"]) != 3 {
		t.Fatal("later capture lost prior revision")
	}
}
func TestAggregateRolloverFailurePublishesNeitherPart(t *testing.T) {
	for _, phase := range []string{"before-reserve", "after-reserve", "before-commit"} {
		t.Run(phase, func(t *testing.T) {
			s, _ := fresh(t)
			must(t, s.AddToday("pending"))
			before := s.Snapshot()
			wire, e := canonical.Marshal(s.snapshot)
			must(t, e)
			next, _ := testDay.AddDays(1)
			s.disk.fault = func(p string) error {
				if p == phase {
					return ErrIncomplete
				}
				return nil
			}
			if e = s.AdvanceTo(next); e == nil {
				t.Fatal("fault accepted")
			}
			after, e := canonical.Marshal(s.snapshot)
			must(t, e)
			if !reflect.DeepEqual(before, s.Snapshot()) || string(wire) != string(after) || s.Today() != testDay {
				t.Fatal("task/audit/today partially published")
			}
		})
	}
}
func TestAggregateRejectsZeroedAuditDespiteCodecShortcut(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("pending"))
	next, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(next))
	copy, e := inspectEmptyHistory(s.snapshot)
	must(t, e)
	extra, e := emptySupplement()
	must(t, e)
	copy.Object["classifications"] = extra.Classifications
	// The generic candidate codec's empty audit shortcut is intentionally not
	// sufficient for the development aggregate's stronger lifecycle invariants.
	forged, e := v7.New(v7.Options{}).Encode(copy)
	must(t, e)
	if _, e = decodeSnapshot(forged); e == nil {
		t.Fatal("load skipped empty-history validation")
	}
	if _, e = encodeSnapshot(copy); e == nil {
		t.Fatal("save skipped empty-history validation")
	}
	revision := s.disk.revision
	if e = s.disk.commit(copy); e == nil || s.disk.revision != revision {
		t.Fatal("invalid audit reserved a revision")
	}
}
func TestLegacyDevelopmentIdentityRequiresCleanCut(t *testing.T) {
	s, p := fresh(t)
	s.Close()
	must(t, os.WriteFile(filepath.Join(p, markerName), []byte("noonmark-mygo-development-session-v1\n"), 0600))
	before, _ := os.ReadDir(p)
	if r, e := Open(p, testDay); !errors.Is(e, ErrSession) {
		if r != nil {
			r.Close()
		}
		t.Fatal(e)
	}
	after, _ := os.ReadDir(p)
	if len(before) != len(after) {
		t.Fatal("legacy session migrated or altered")
	}
}
func TestAggregateBackwardsDayDoesNotUnlockHistory(t *testing.T) {
	s, _ := fresh(t)
	must(t, s.AddToday("pending"))
	next, _ := testDay.AddDays(1)
	must(t, s.AdvanceTo(next))
	wire, e := encodeSnapshot(s.snapshot)
	must(t, e)
	must(t, s.AdvanceTo(testDay))
	after, e := encodeSnapshot(s.snapshot)
	must(t, e)
	if string(wire) != string(after) {
		t.Fatal("backwards calendar sample rewrote history")
	}
	if e = s.AddToday("cannot use locked day"); !errors.Is(e, core.ErrImmutableHistory) {
		t.Fatal(e)
	}
}
