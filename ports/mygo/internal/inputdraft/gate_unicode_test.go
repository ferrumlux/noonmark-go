// SPDX-License-Identifier: AGPL-3.0-only
package inputdraft

import "testing"

func equivalentOwners() [][2]string {
	return [][2]string{
		{"合成:任务:caf\u00e9", "合成:任务:cafe\u0301"},
		{"合成:任务:cafe\u0301", "合成:任务:caf\u00e9"},
		{"synthetic:\uac00", "synthetic:\u1100\u1161"},
		{"synthetic:\u1100\u1161", "synthetic:\uac00"},
	}
}

func TestCanonicalScheduleOwnerMatchesWithoutChangingBytes(t *testing.T) {
	for _, pair := range equivalentOwners() {
		g, err := New(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		s := mustSchedule(t, g.DraftDidChange())
		if s.OwnerID != pair[0] || g.OwnerID() != pair[0] {
			t.Fatal("construction or schedule normalized stored owner")
		}
		s.OwnerID = pair[1]
		if !g.PermitsAutosave(&s) {
			t.Fatalf("canonical schedule owner rejected: %q -> %q", pair[0], pair[1])
		}
		r := mustRequest(t, g.BeginPersistence(s))
		if r.OwnerID != pair[0] || g.OwnerID() != pair[0] || s.OwnerID != pair[1] {
			t.Fatal("comparison changed request, gate or supplied token bytes")
		}
	}
}

func TestCanonicalRequestOwnerMatchesBothCallbacks(t *testing.T) {
	for _, pair := range equivalentOwners() {
		for _, success := range []bool{false, true} {
			g, err := New(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			r := mustRequest(t, g.BeginPersistence(mustSchedule(t, g.DraftDidChange())))
			r.OwnerID = pair[1]
			if success {
				if g.PersistenceDidSucceed(r) != nil || g.HasUnflushedChanges() || g.HasPersistenceInFlight() {
					t.Fatal("canonical success request rejected")
				}
			} else {
				s := mustSchedule(t, g.PersistenceDidFail(r))
				if s.OwnerID != pair[0] || s.DelayMilliseconds != 1000 || g.HasPersistenceInFlight() {
					t.Fatal("canonical failure request rejected or rewrote owner")
				}
			}
			if g.OwnerID() != pair[0] || r.OwnerID != pair[1] {
				t.Fatal("callback changed original bytes")
			}
		}
	}
}

func TestDifferentUnicodeOwnersAndNumericIdentitiesReject(t *testing.T) {
	for _, owner := range []string{"合成:任务:cafe", "合成:任务:caf\u00e8", "合成:任务:CAF\u00c9"} {
		g, err := New("合成:任务:caf\u00e9")
		if err != nil {
			t.Fatal(err)
		}
		s := mustSchedule(t, g.DraftDidChange())
		original := s
		s.OwnerID = owner
		if g.PermitsAutosave(&s) || g.BeginPersistence(s) != nil {
			t.Fatal("different Unicode owner permitted")
		}
		r := mustRequest(t, g.BeginPersistence(original))
		r.OwnerID = owner
		before := *g
		if g.PersistenceDidSucceed(r) != nil || g.PersistenceDidFail(r) != nil || *g != before {
			t.Fatal("different Unicode callback owner accepted")
		}
	}
	for _, field := range []string{"generation", "revision", "request_id"} {
		g, err := New("caf\u00e9")
		if err != nil {
			t.Fatal(err)
		}
		r := mustRequest(t, g.BeginPersistence(mustSchedule(t, g.DraftDidChange())))
		r.OwnerID = "cafe\u0301"
		switch field {
		case "generation":
			r.Generation++
		case "revision":
			r.Revision++
		case "request_id":
			r.RequestID++
		}
		before := *g
		if g.PersistenceDidSucceed(r) != nil || g.PersistenceDidFail(r) != nil || *g != before {
			t.Fatal("canonical owner must not bypass numeric identity")
		}
	}
}
