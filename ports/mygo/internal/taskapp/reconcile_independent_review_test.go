// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func TestReconcileReviewLastRepresentableSettlement(t *testing.T) {
	s, path := fresh(t)
	must(t, s.AddToday("最后可用时钟"))
	before := s.Snapshot()
	tr := before.Traces[0]
	now, err := core.InstantFromSeconds(math.Nextafter(math.MaxFloat64, 0))
	must(t, err)
	cmd := core.SetManualProgressInput{ChainID: tr.ChainID, TraceID: tr.ID, Percent: 40, Today: testDay, Now: now}
	k, err := core.NewTaskKernel(before)
	must(t, err)
	must(t, k.SetManualProgress(cmd))
	wire, err := captureManualProgressHistory(s.snapshot, before, k.Snapshot(), cmd)
	must(t, err)
	wire, graph, err := normalizeSnapshot(wire)
	must(t, err)
	must(t, s.disk.commit(wire))
	s.graph, s.snapshot = graph, wire
	revision := s.disk.revision
	s.clock = func() time.Time { return time.Unix(978307201, 0) }
	tomorrow, err := testDay.AddDays(1)
	must(t, err)
	must(t, s.AdvanceTo(tomorrow))
	maximum, err := core.InstantFromSeconds(math.MaxFloat64)
	must(t, err)
	after := s.Snapshot()
	if after.Traces[0].ContentUpdatedAt != maximum || after.Days[0].UpdatedAt != maximum || s.disk.revision != revision+1 {
		t.Fatal("last representable instant not used exactly once")
	}
	must(t, ValidateEmptyHistory(s.snapshot))
	must(t, s.Close())
	r, err := Open(path, tomorrow)
	must(t, err)
	defer r.Close()
	r.clock = func() time.Time { t.Fatal("settled history sampled clock"); return time.Time{} }
	later, err := tomorrow.AddDays(1)
	must(t, err)
	must(t, r.AdvanceTo(later))
	if !reflect.DeepEqual(after, r.Snapshot()) || r.disk.revision != revision+1 || r.Today() != later {
		t.Fatal("locked exhausted history changed")
	}
}

func TestReconcileReviewIncompleteSuccessorNoop(t *testing.T) {
	s, path := fresh(t)
	must(t, s.AddToday("不完整后继"))
	installReconcileMaximum(t, s)
	before, rev := s.Snapshot(), s.disk.revision
	wire, err := encodeSnapshot(s.snapshot)
	must(t, err)
	must(t, os.Mkdir(filepath.Join(path, "rev-0000000004"), 0700))
	s.clock = func() time.Time { t.Fatal("incomplete no-op sampled clock"); return time.Time{} }
	if err := s.AdvanceTo(testDay); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	after, err := encodeSnapshot(s.snapshot)
	must(t, err)
	if s.Today() != testDay || s.disk.revision != rev || !reflect.DeepEqual(before, s.Snapshot()) || string(wire) != string(after) {
		t.Fatal("incomplete no-op published")
	}
}
