// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"testing"
)

func ordinaryWriterCandidate(t *testing.T, s *Service) persistableOrdinaryCandidate {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, e := newIDs(2)
	must(t, e)
	now, e := s.nextInstant()
	must(t, e)
	c, e := s.taskCandidateLocked(ordinaryTransition{kind: ordinaryAdd, add: core.CreatePoolTaskInput{ChainID: core.TaskChainID(ids[0]), DefinitionID: core.TaskDefinitionID(ids[1]), Title: "writer candidate", Now: now}})
	must(t, e)
	return c
}
func TestOrdinaryWriterDirectRejectsTamperedBindings(t *testing.T) {
	for _, kind := range []string{"owner", "cloned-before", "other-before", "head", "revision", "edge", "after", "bytes", "both", "neither"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := freshOrdinary(t)
			c := ordinaryWriterCandidate(t, s)
			before := s.disk.revision
			switch kind {
			case "owner":
				c.owner = &serviceLineage{epoch: 99}
			case "cloned-before":
				a := *c.before
				c.before = &a
			case "other-before":
				r, _ := freshOrdinary(t)
				c.before = r.aggregate
			case "head":
				c.head = "bad"
			case "revision":
				c.revision++
			case "edge":
				c.task.input.add.Title = "different edge"
			case "after":
				c.after = *s.aggregate
			case "bytes":
				c.bytes = append(c.bytes, ' ')
			case "both":
				c.category = &ordinaryCategoryEdge{}
			case "neither":
				c.task = nil
			}
			reserved := false
			s.disk.fault = func(stage string) error {
				if stage == "before-reserve" {
					reserved = true
				}
				return nil
			}
			o := ordinaryDirectWriter(s, c)
			if o.err == nil || o.extent != WriteNone || s.disk.revision != before || reserved {
				t.Fatalf("writer accepted %s: %+v", kind, o)
			}
		})
	}
}
func TestOrdinaryWriterDirectOrderAndHead(t *testing.T) {
	s, path := freshOrdinary(t)
	c := ordinaryWriterCandidate(t, s)
	old := s.aggregate
	before := s.disk.revision
	o := ordinaryDirectWriter(s, c)
	must(t, o.err)
	if o.extent != WriteCommittedConfirmed || s.disk.revision != before+1 || s.disk.ordinaryHead == old {
		t.Fatal(o)
	}
	// Direct writer does not publish Service. Rewriting c's head cannot turn the
	// still-old Service before into the writer's newly confirmed aggregate.
	c.head = s.disk.hash
	c.revision = s.disk.revision
	if o = ordinaryDirectWriter(s, c); o.err == nil || o.extent != WriteNone {
		t.Fatal("old before accepted after direct write", o)
	}
	next := ordinaryWriterCandidate(t, s)
	if o = ordinaryDirectWriter(s, next); o.err == nil || o.extent != WriteNone {
		t.Fatal("unpublished service state authorized another write", o)
	}
	must(t, s.Close())
	r, e := OpenOrdinaryDevelopment(path, testDay)
	must(t, e)
	defer r.Close()
	if len(r.Snapshot().Chains) != 1 {
		t.Fatal("confirmed direct write not restored")
	}
	if o = ordinaryDirectWriter(r, c); o.err == nil {
		t.Fatal("fresh owner accepted old capability")
	}
}
func TestOrdinaryWriterNoLegacyOrRawInputBypass(t *testing.T) {
	s, _ := freshOrdinary(t)
	before := s.disk.revision
	c := ordinaryWriterCandidate(t, s)
	for _, in := range []storeWriteInput{{}, {legacy: &s.snapshot}, {legacy: &s.snapshot, ordinary: &c}} {
		o := ordinaryDirectStoreInput(s, in)
		if o.err == nil || o.extent != WriteNone || s.disk.revision != before {
			t.Fatal("untyped/legacy bypass", o)
		}
	}
	other, _ := freshOrdinary(t)
	o := ordinaryDirectWriter(other, c)
	if o.err == nil {
		t.Fatal("foreign Service candidate accepted")
	}
	must(t, s.Close())
	o = ordinaryDirectWriter(s, c)
	if o.err == nil || o.extent != WriteNone {
		t.Fatal("closed owner accepted", o)
	}
}
func TestOrdinaryWriterDirectUncertainInvalidatesService(t *testing.T) {
	for _, stage := range []string{"before-reserve", "after-reserve", "before-commit", "after-persist"} {
		t.Run(stage, func(t *testing.T) {
			s, path := freshOrdinary(t)
			c := ordinaryWriterCandidate(t, s)
			s.disk.fault = func(at string) error {
				if at == stage {
					return errors.New("synthetic direct writer fault")
				}
				return nil
			}
			o := ordinaryDirectWriter(s, c)
			if o.err == nil {
				t.Fatal("fault ignored")
			}
			if stage == "before-reserve" {
				if s.health.Session != SessionWritable || o.extent != WriteNone {
					t.Fatal(o)
				}
				s.disk.fault = nil
				must(t, ordinaryDirectPersist(s, c))
				return
			}
			if s.health.Session != SessionNeedsReopen || !s.disk.blocked {
				t.Fatal("direct uncertainty did not invalidate bound Service", o, s.health)
			}
			if o = ordinaryDirectWriter(s, c); o.err == nil {
				t.Fatal("uncertain writer resumed")
			}
			must(t, s.Close())
			r, e := OpenOrdinaryDevelopment(path, testDay)
			if stage == "after-persist" {
				must(t, e)
				r.Close()
			} else if e == nil {
				r.Close()
				t.Fatal("partial tail silently admitted")
			}
		})
	}
}

func ordinaryDirectWriter(s *Service, c persistableOrdinaryCandidate) commitObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disk.commitOrdinaryObserved(c)
}
func ordinaryDirectStoreInput(s *Service, in storeWriteInput) commitObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disk.commitStoreInputObserved(in)
}
func ordinaryDirectPersist(s *Service, c persistableOrdinaryCandidate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistOrdinaryLocked(c)
}
