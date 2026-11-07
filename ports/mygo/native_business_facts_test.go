package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"github.com/ferrumlux/noonmark/ports/mygo/internal/taskapp"
)

// This is synthetic Service evidence, not a native window or cross-process test.
func nativeBusinessFixture(t *testing.T) (taskapp.OrdinaryReadView, taskapp.OrdinaryReadView, string) {
	t.Helper()
	today := core.LocalDate{Year: 2026, Month: 6, Day: 12}
	path := filepath.Join(t.TempDir(), "session")
	s, err := taskapp.CreateOrdinaryDevelopment(path, today)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	initial, err := s.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if err = validateNativeBusinessEmpty(initial, today); err != nil {
		t.Fatal(err)
	}
	if err = s.AddToday(nativeBusinessTaskTitle); err != nil {
		t.Fatal(err)
	}
	added, err := s.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	chain, err := validateNativeBusinessTask(added, today)
	if err != nil {
		t.Fatal(err)
	}
	if added.StoreRevision != initial.StoreRevision+1 || !reflect.DeepEqual(added.Classification, initial.Classification) {
		t.Fatal("AddToday exact revision/classification contract")
	}
	interaction, err := core.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	out := s.ReplaceCategorySelectionOn(today, chain, ordinary.Choice{Kind: ordinary.ChoiceNew, Name: nativeBusinessCategoryName, ColorHex: nativeBusinessCategoryColor}, ordinary.UUID(interaction))
	if out.Err != nil || out.Preparation.Err != nil || out.Result != taskapp.ResultConfirmed || out.WriteExtent != taskapp.WriteCommittedConfirmed || out.Session != taskapp.SessionWritable || out.Receipt == nil {
		t.Fatalf("category outcome: %+v", out)
	}
	final, err := s.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = validateNativeBusinessFinal(final, today); err != nil {
		b, _ := json.Marshal(final.Classification)
		t.Fatalf("%v; actual classification=%s", err, b)
	}
	if final.StoreRevision != initial.StoreRevision+2 || !reflect.DeepEqual(final.Graph, added.Graph) {
		t.Fatal("ChoiceNew exact +2/unchanged graph contract")
	}
	r := final.Classification.ChangeRecords[0]
	if r.InteractionID != ordinary.UUID(interaction) || r.PlanID != out.PlanID || r.ID != out.Receipt.ChangeRecordID || r.DecisionID != out.Receipt.DecisionID || r.IntegrityDigest != out.Receipt.ChangeRecordIntegrityDigest {
		t.Fatal("actual command receipt binding")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	return initial, final, path
}
func TestNativeBusinessFixtureExactColdOpen(t *testing.T) {
	_, final, path := nativeBusinessFixture(t)
	reopened, err := taskapp.OpenOrdinaryDevelopment(path, final.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cold, err := reopened.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = validateNativeBusinessFinal(cold, final.Today); err != nil {
		t.Fatal(err)
	}
	a, ah, err := nativeBusinessFactsJSON(final)
	if err != nil {
		t.Fatal(err)
	}
	b, bh, err := nativeBusinessFactsJSON(cold)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("persistent_facts_json=%s", a)
	if a != b || ah != bh || cold.StoreRevision != final.StoreRevision || cold.Today != final.Today {
		t.Fatal("cold-open facts/revision differ")
	}
	// The uint64 integer survives exact JSON decoding, without float64.
	var facts PersistentFactsV1
	if err = json.Unmarshal([]byte(a), &facts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(facts, PersistentFactsV1{1, final.Graph, final.Classification}) {
		t.Fatal("DTO round trip changed fields or bits")
	}
	if !strings.Contains(a, `"Version":1`) || !strings.Contains(a, `"Subtasks":[]`) {
		t.Fatal("fixed DTO schema")
	}
}
func TestNativeBusinessFixtureRejectsCrossDayOpen(t *testing.T) {
	_, final, path := nativeBusinessFixture(t)
	next, err := final.Today.AddDays(1)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := taskapp.OpenOrdinaryDevelopment(path, next)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	changed, err := reopened.ReadView()
	if err != nil {
		t.Fatal(err)
	}
	if changed.StoreRevision <= final.StoreRevision {
		t.Fatal("cross-day Open expected settlement write")
	}
	if _, err = validateNativeBusinessFinal(changed, final.Today); err == nil {
		t.Fatal("cross-day Open accepted old date")
	}
	if _, err = validateNativeBusinessFinal(changed, next); err == nil {
		t.Fatal("cross-day settled facts accepted as fixed scenario")
	}
}
func TestNativeBusinessFactsRejectMutations(t *testing.T) {
	_, final, _ := nativeBusinessFixture(t)
	cases := map[string]func(*taskapp.OrdinaryReadView){
		"nil-subtasks": func(v *taskapp.OrdinaryReadView) { v.Graph.Subtasks = nil },
		"wrong-title":  func(v *taskapp.OrdinaryReadView) { v.Graph.Definitions[0].Title = "other" },
		"priority":     func(v *taskapp.OrdinaryReadView) { v.Graph.Traces[0].Priority = 0 },
		"trace-status": func(v *taskapp.OrdinaryReadView) { v.Graph.Traces[0].Status = core.TraceCompleted },
		"definition-link": func(v *taskapp.OrdinaryReadView) {
			v.Graph.Traces[0].DefinitionID = "00000000-0000-0000-0000-000000000001"
		},
		"history-nil": func(v *taskapp.OrdinaryReadView) { v.Classification.RelationHistory = nil },
		"missing-receipt": func(v *taskapp.OrdinaryReadView) {
			v.Classification.CommittedReceiptsByInteractionID = map[ordinary.UUID]ordinary.Receipt{}
		},
		"revision":    func(v *taskapp.OrdinaryReadView) { v.Classification.Revision = 2 },
		"change-kind": func(v *taskapp.OrdinaryReadView) { v.Classification.ChangeRecords[0].Changes[0].Kind = "create" },
		"receipt-decision": func(v *taskapp.OrdinaryReadView) {
			for id, r := range v.Classification.CommittedReceiptsByInteractionID {
				r.DecisionID = "00000000-0000-0000-0000-000000000001"
				v.Classification.CommittedReceiptsByInteractionID[id] = r
			}
		},
		"health": func(v *taskapp.OrdinaryReadView) { v.Health.Session = taskapp.SessionClosed },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(final)
			if err != nil {
				t.Fatal(err)
			}
			var v taskapp.OrdinaryReadView
			if err = json.Unmarshal(raw, &v); err != nil {
				t.Fatal(err)
			}
			mutate(&v)
			if _, err = validateNativeBusinessFinal(v, final.Today); err == nil {
				t.Fatal("mutated fixture accepted")
			}
		})
	}
}
func TestNativeBusinessInstantBits(t *testing.T) {
	// Negative seconds have larger uint64 bits than positive seconds. Comparison
	// must use Seconds, while serialization must preserve the original bits.
	a, _ := core.InstantFromSeconds(-1)
	b, _ := core.InstantFromSeconds(1)
	if uint64(a) <= uint64(b) || !a.Before(b) {
		t.Fatal("test precondition")
	}
	raw, err := json.Marshal([]core.Instant{a, b})
	if err != nil {
		t.Fatal(err)
	}
	var got []core.Instant
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []core.Instant{a, b}) {
		t.Fatal("lost uint64 precision")
	}
	if _, err = json.Marshal(core.Instant(0x7ff0000000000000)); err == nil {
		t.Fatal("nonfinite accepted")
	}
}
