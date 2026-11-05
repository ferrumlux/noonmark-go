// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenHandleRejectsHistoricalCorruptionBeforeReservation(t *testing.T) {
	for _, kind := range []string{"latest-payload", "earlier-payload", "earlier-marker", "identity-removed", "identity-changed", "root-moved"} {
		t.Run(kind, func(t *testing.T) {
			s, p := fresh(t)
			must(t, s.AddPool("first"))
			before := s.Snapshot()
			revision := s.disk.revision
			switch kind {
			case "latest-payload", "earlier-payload":
				n := "rev-0000000002"
				if kind == "earlier-payload" {
					n = "rev-0000000001"
				}
				file := filepath.Join(p, n, "state.json")
				b, e := os.ReadFile(file)
				must(t, e)
				b[len(b)-1] = '!'
				must(t, os.WriteFile(file, b, 0600))
			case "earlier-marker":
				must(t, os.WriteFile(filepath.Join(p, "rev-0000000001/COMMITTED"), []byte("corrupted"), 0600))
			case "identity-removed":
				must(t, os.Remove(filepath.Join(p, markerName)))
			case "identity-changed":
				must(t, os.WriteFile(filepath.Join(p, markerName), []byte("foreign"), 0600))
			case "root-moved":
				must(t, os.Rename(p, p+"-moved"))
				p += "-moved"
			}
			entries, _ := os.ReadDir(p)
			if e := s.AddPool("must not publish"); e == nil {
				t.Fatal("successful save into unreopenable session")
			}
			after, _ := os.ReadDir(p)
			if len(entries) != len(after) || s.disk.revision != revision || !reflect.DeepEqual(before, s.Snapshot()) {
				t.Fatal("failed integrity check reserved or published")
			}
		})
	}
}
func TestLateHistoricalCorruptionPreventsPublication(t *testing.T) {
	s, p := fresh(t)
	must(t, s.AddPool("first"))
	before := s.Snapshot()
	s.disk.fault = func(phase string) error {
		if phase == "before-commit" {
			return os.WriteFile(filepath.Join(p, markerName), []byte("changed during write"), 0600)
		}
		return nil
	}
	if e := s.AddPool("must not publish"); e == nil {
		t.Fatal("reported success after late identity loss")
	}
	if !reflect.DeepEqual(before, s.Snapshot()) || !s.disk.blocked {
		t.Fatal("uncertain commit published or continued")
	}
	if e := s.AddPool("retry"); e == nil {
		t.Fatal("uncertain handle accepted retry")
	}
}

func TestLaterWriterBeforeAcknowledgmentDoesNotCauseFalseFailure(t *testing.T) {
	s, p := fresh(t)
	s.disk.fault = func(phase string) error {
		if phase != "after-persist" {
			return nil
		}
		other, e := Open(p, testDay)
		if e != nil {
			return e
		}
		defer other.Close()
		return other.AddPool("second writer")
	}
	must(t, s.AddPool("first writer"))
	if len(s.Snapshot().Chains) != 1 {
		t.Fatal("own committed candidate not published")
	}
	r, e := Open(p, testDay)
	must(t, e)
	defer r.Close()
	if len(r.Snapshot().Chains) != 2 {
		t.Fatal("successor was lost")
	}
	s.disk.fault = nil
	if e = s.AddPool("stale retry"); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale handle should conflict: %v", e)
	}
}
