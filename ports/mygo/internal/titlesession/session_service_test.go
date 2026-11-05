// SPDX-License-Identifier: AGPL-3.0-only
package titlesession_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/titlesession"
)

func serviceSession(t *testing.T, title string) (*taskapp.Service, *titlesession.Session, string, core.LocalDate) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-session")
	day := core.LocalDate{Year: 2026, Month: 11, Day: 5}
	svc, err := taskapp.Create(path, day)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	if err = svc.AddToday(title); err != nil {
		t.Fatal(err)
	}
	tr := svc.Snapshot().Traces[0]
	s, err := titlesession.New(titlesession.Config{OwnerID: "task:" + string(tr.ChainID) + ":title", SessionID: 1, Generation: 1, TraceID: string(tr.ID), PersistedText: title})
	if err != nil {
		t.Fatal(err)
	}
	return svc, s, path, day
}
func serviceSave(t *testing.T, svc *taskapp.Service, s *titlesession.Session, text string) titlesession.Update {
	t.Helper()
	u := s.RequestLifecycleFlush(s.Identity(), titlesession.NativeSnapshot{Text: text}, true)
	if u.Job == nil {
		t.Fatal("missing save candidate")
	}
	b := s.BeginPersistence(*u.Job)
	if b.Request == nil {
		t.Fatal("missing immutable request")
	}
	actual, err := svc.SaveTraceTitleInputAcknowledged(core.DayTraceID(b.Request.TraceID()), b.Request.NormalizedText())
	result := titlesession.PersistenceResult{Err: err}
	if err == nil {
		result.VerifiedText = titlesession.OptionalText{Text: actual, Present: true}
	}
	return s.PersistenceDidComplete(*b.Request, result)
}
func TestSessionAcknowledgmentMatchesServiceAndReopenedStore(t *testing.T) {
	svc, s, path, day := serviceSession(t, "原题")
	u := serviceSave(t, svc, s, " 新标题\r\n第二行 ")
	if !u.SnapshotIsFlushed() || u.State.LastAcknowledgedText != "新标题\n第二行" || !u.DraftReplacement.Present {
		t.Fatalf("unverified receipt: %+v", u)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.Open(path, day)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Snapshot().Definitions[0].Title != u.State.LastAcknowledgedText {
		t.Fatal("receipt differs from reopened bytes")
	}
}
func TestSessionCanonicalNoopUsesServiceExistingBytes(t *testing.T) {
	svc, s, _, _ := serviceSession(t, "café")
	// Edge whitespace makes this a dirty draft, while normalized text remains
	// canonically equal to the Service's existing composed representation.
	u := serviceSave(t, svc, s, " cafe\u0301 ")
	if !u.SnapshotIsFlushed() || u.State.LastAcknowledgedText != "café" || u.State.Draft != "café" {
		t.Fatalf("invented canonical receipt: %+v", u)
	}
	if svc.Snapshot().Definitions[0].Title != "café" {
		t.Fatal("no-op changed stored bytes")
	}
}
func TestSessionRejectedServiceWriteKeepsDraftAndCancelsLifecycle(t *testing.T) {
	for _, kind := range []string{"closed", "completed", "settled"} {
		t.Run(kind, func(t *testing.T) {
			svc, s, _, day := serviceSession(t, "原题")
			tr := svc.Snapshot().Traces[0].ID
			var err error
			switch kind {
			case "closed":
				err = svc.Close()
			case "completed":
				err = svc.SetCompleted(tr, true)
			case "settled":
				next, e := day.AddDays(1)
				if e != nil {
					t.Fatal(e)
				}
				err = svc.AdvanceTo(next)
			}
			if err != nil {
				t.Fatal(err)
			}
			u := serviceSave(t, svc, s, " 保留未保存标题 ")
			if u.Err == nil || u.SnapshotIsFlushed() || u.State.Draft != " 保留未保存标题 " || u.State.LastAcknowledgedText != "原题" || !u.State.Dirty || u.State.Lifecycle.Status != titlesession.LifecycleFailed {
				t.Fatalf("lost rejected draft: %+v", u)
			}
			if kind == "closed" && !errors.Is(u.Err, taskapp.ErrSession) {
				t.Fatalf("unexpected closed error: %v", u.Err)
			}
			if s.Dispose(s.Identity()).Accepted {
				t.Fatal("disposed rejected title")
			}
		})
	}
}
