// SPDX-License-Identifier: AGPL-3.0-only
package inputdraft

import (
	"strings"
	"testing"
)

func TestStreamSafeCollisionCannotRouteAnotherOwner(t *testing.T) {
	a := strings.Repeat("\u0301", 31)
	b := strings.Repeat("\u0301", 30) + "\u034f\u0301"
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		t.Run("schedule", func(t *testing.T) {
			g, err := New(pair[0])
			if err != nil {
				t.Fatal(err)
			}
			s := mustSchedule(t, g.DraftDidChange())
			s.OwnerID = pair[1]
			before := *g
			if g.PermitsAutosave(&s) || g.BeginPersistence(s) != nil || *g != before {
				t.Fatal("distinct CGJ owner schedule accepted")
			}
			if g.OwnerID() != pair[0] || s.OwnerID != pair[1] {
				t.Fatal("owner bytes rewritten")
			}
		})
		for _, success := range []bool{false, true} {
			t.Run("callback", func(t *testing.T) {
				g, err := New(pair[0])
				if err != nil {
					t.Fatal(err)
				}
				r := mustRequest(t, g.BeginPersistence(mustSchedule(t, g.DraftDidChange())))
				wrong := r
				wrong.OwnerID = pair[1]
				before := *g
				if success {
					g.PersistenceDidSucceed(wrong)
				} else {
					g.PersistenceDidFail(wrong)
				}
				if *g != before {
					t.Fatal("distinct CGJ owner callback changed gate")
				}
				if g.OwnerID() != pair[0] || r.OwnerID != pair[0] || wrong.OwnerID != pair[1] {
					t.Fatal("callback normalized original bytes")
				}
				g.PersistenceDidSucceed(r)
				if g.HasPersistenceInFlight() || g.HasUnflushedChanges() {
					t.Fatal("correct owner could not complete")
				}
			})
		}
	}
}

func TestInvalidUTF8OwnerFailsClosed(t *testing.T) {
	for _, owner := range []string{string([]byte{0xff}), "owner" + string([]byte{0xc3})} {
		g, err := New(owner)
		if err == nil {
			token := mustSchedule(t, g.DraftDidChange())
			before := *g
			if g.PermitsAutosave(&token) || g.BeginPersistence(token) != nil || *g != before {
				t.Fatal("invalid UTF-8 owner matched identical malformed bytes")
			}
			if g.OwnerID() != owner || token.OwnerID != owner {
				t.Fatal("invalid owner bytes rewritten")
			}
		}
		valid, err := New("valid")
		if err != nil {
			t.Fatal(err)
		}
		request := mustRequest(t, valid.BeginPersistence(mustSchedule(t, valid.DraftDidChange())))
		bad := request
		bad.OwnerID = owner
		before := *valid
		valid.PersistenceDidSucceed(bad)
		valid.PersistenceDidFail(bad)
		if *valid != before {
			t.Fatal("malformed callback altered valid owner state")
		}
		valid.PersistenceDidSucceed(request)
		if valid.HasUnflushedChanges() || valid.HasPersistenceInFlight() {
			t.Fatal("rejected malformed callback consumed real request")
		}
	}
}
