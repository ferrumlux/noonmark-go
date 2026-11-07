// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

type ordinaryOwnerPublishedState struct {
	aggregate *admittedAggregate
	graph     core.TaskGraph
	snapshot  []byte
	revision  int
	hash      string
	head      *admittedAggregate
}

func ordinaryOwnerPublished(t *testing.T, s *Service) ordinaryOwnerPublishedState {
	t.Helper()
	wire, e := canonical.Marshal(s.snapshot)
	must(t, e)
	return ordinaryOwnerPublishedState{s.aggregate, s.Snapshot(), wire, s.disk.revision, s.disk.hash, s.disk.ordinaryHead}
}
func ordinaryOwnerRequireUnpublished(t *testing.T, s *Service, before ordinaryOwnerPublishedState) {
	t.Helper()
	after := ordinaryOwnerPublished(t, s)
	if before.aggregate != after.aggregate || before.head != after.head || before.revision != after.revision || before.hash != after.hash || !reflect.DeepEqual(before.graph, after.graph) || !bytes.Equal(before.snapshot, after.snapshot) {
		t.Fatal("failed owner attempt changed published graph/snapshot/aggregate or disk head")
	}
}
func ordinaryOwnerPersistAttempt(s *Service, c persistableOrdinaryCandidate, a *attempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistOrdinaryLocked(c, a)
}
func TestOrdinaryOwnerRuntimeRejectionPreservesState(t *testing.T) {
	for _, kind := range []string{"closed", "needs-reopen", "owner", "before", "head", "revision", "edge", "both edges", "no edge"} {
		t.Run(kind, func(t *testing.T) {
			s, path := freshOrdinary(t)
			c := ordinaryWriterCandidate(t, s)
			switch kind {
			case "closed":
				must(t, s.Close())
			case "needs-reopen":
				s.mu.Lock()
				s.invalidateLocked(HealthWriteUncertain, StageReadback)
				s.mu.Unlock()
			case "owner":
				c.owner = &serviceLineage{epoch: 99}
			case "before":
				copy := *c.before
				c.before = &copy
			case "head":
				c.head = "wrong head"
			case "revision":
				c.revision++
			case "edge":
				c.task.input.add.Title = "different retained command"
			case "both edges":
				c.category = &ordinaryCategoryEdge{}
			case "no edge":
				c.task = nil
			}
			before := ordinaryOwnerPublished(t, s)
			disk := outcomeDiskBytes(t, path)
			reserved := false
			s.disk.fault = func(stage string) error {
				if stage == "before-reserve" {
					reserved = true
				}
				return nil
			}
			a := newAttempt()
			err := ordinaryOwnerPersistAttempt(s, c, &a)
			if err == nil || a.extent != WriteNone || reserved {
				t.Fatalf("rejection failed: err=%v attempt=%+v reserve=%v", err, a, reserved)
			}
			ordinaryOwnerRequireUnpublished(t, s, before)
			if !reflect.DeepEqual(disk, outcomeDiskBytes(t, path)) {
				t.Fatal("rejection changed disk bytes")
			}
		})
	}
}
func TestOrdinaryOwnerRuntimeObservationAndPublication(t *testing.T) {
	for _, stage := range []string{"before-reserve", "after-reserve", "after-persist"} {
		t.Run(stage, func(t *testing.T) {
			s, path := freshOrdinary(t)
			c := ordinaryWriterCandidate(t, s)
			before := ordinaryOwnerPublished(t, s)
			diskBefore := outcomeDiskBytes(t, path)
			s.disk.fault = func(at string) error {
				if at == stage {
					return errors.New("synthetic owner fault")
				}
				return nil
			}
			a := newAttempt()
			err := ordinaryOwnerPersistAttempt(s, c, &a)
			if err == nil {
				t.Fatal("fault ignored")
			}
			ordinaryOwnerRequireUnpublished(t, s, before)
			wantExtent, wantStage := WriteNone, StageReserve
			if stage == "after-reserve" {
				wantExtent = WriteReservedOrPartial
			}
			if stage == "after-persist" {
				wantExtent = WriteWrittenUnacknowledged
				wantStage = StagePostPersist
			}
			if a.extent != wantExtent || a.stage != wantStage {
				t.Fatalf("observation lost: %+v", a)
			}
			if stage == "before-reserve" {
				if s.health.Session != SessionWritable || !reflect.DeepEqual(diskBefore, outcomeDiskBytes(t, path)) {
					t.Fatal("pre-reserve fault invalidated or wrote")
				}
				s.disk.fault = nil
				a = newAttempt()
				must(t, ordinaryOwnerPersistAttempt(s, c, &a))
				if a.extent != WriteCommittedConfirmed || a.stage != StageNone || s.aggregate != s.disk.ordinaryHead {
					t.Fatal("healthy retry did not publish confirmed writer head")
				}
			} else {
				if s.health.Session != SessionNeedsReopen || s.health.Reason != HealthWriteUncertain || s.health.FailureStage != wantStage || !s.disk.blocked {
					t.Fatalf("sticky invalidation lost: %+v", s.health)
				}
				disk := outcomeDiskBytes(t, path)
				s.disk.fault = nil
				a = newAttempt()
				if err = ordinaryOwnerPersistAttempt(s, c, &a); !errors.Is(err, ErrIncomplete) || a.extent != WriteNone {
					t.Fatal("uncertain owner resumed", err, a)
				}
				if !reflect.DeepEqual(disk, outcomeDiskBytes(t, path)) {
					t.Fatal("subsequent rejected write changed failed tail")
				}
				ordinaryOwnerRequireUnpublished(t, s, before)
			}
		})
	}
	t.Run("success", func(t *testing.T) {
		s, path := freshOrdinary(t)
		c := ordinaryWriterCandidate(t, s)
		revision := s.disk.revision
		graph, state, e := c.after.document.Facts()
		must(t, e)
		a := newAttempt()
		must(t, ordinaryOwnerPersistAttempt(s, c, &a))
		if a.extent != WriteCommittedConfirmed || a.stage != StageNone || s.aggregate != s.disk.ordinaryHead || s.disk.revision != revision+1 {
			t.Fatal("confirmed publication mismatch", a)
		}
		view, e := s.ReadView()
		must(t, e)
		if !reflect.DeepEqual(graph, view.Graph) || !reflect.DeepEqual(state, view.Classification) || view.StoreRevision != revision+1 || view.Health.Session != SessionWritable {
			t.Fatal("inconsistent successful ReadView")
		}
		must(t, s.Close())
		reopened, e := OpenOrdinaryDevelopment(path, testDay)
		must(t, e)
		defer reopened.Close()
		restored, e := reopened.ReadView()
		must(t, e)
		if !reflect.DeepEqual(view, restored) {
			t.Fatal("confirmed complete ReadView did not reopen")
		}
	})
}
