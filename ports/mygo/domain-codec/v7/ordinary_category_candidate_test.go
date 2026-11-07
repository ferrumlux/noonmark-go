// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

func ordinaryFixture(t testing.TB) (ordinary.OrdinaryContext, ordinary.State, []ordinary.CommittedStep) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	kernel, err := core.NewTaskKernel(core.TaskGraph{})
	must(err)
	description := "原字节 / \\ \" \x00 e\u0301 \u2028 \u2029"
	for i := 1; i <= 2; i++ {
		must(kernel.CreatePoolTask(core.CreatePoolTaskInput{ChainID: core.TaskChainID(testID(i)), DefinitionID: core.TaskDefinitionID(testID(10 + i)), Title: fmt.Sprintf("合成任务%d", i), DescriptionText: &description, Now: core.Instant(math.Float64bits(math.Copysign(0, -1)))}))
	}
	ctx, err := ordinary.NewEmptyContext(kernel.Snapshot())
	must(err)
	state := ordinary.EmptyState()
	var steps []ordinary.CommittedStep
	choices := []ordinary.Choice{
		{Kind: ordinary.ChoiceNew, Name: "工程/\\\"e\u0301", ColorHex: "#123456"},
		{Kind: ordinary.ChoiceExisting, CategoryID: ordinary.CategoryID(testID(300))},
		{Kind: ordinary.ChoiceNone}, {Kind: ordinary.ChoiceNone}, {Kind: ordinary.ChoiceNone},
		{Kind: ordinary.ChoiceExisting, CategoryID: ordinary.CategoryID(testID(300))},
		{Kind: ordinary.ChoiceNew, Name: "工程/\\\"e\u0301", ColorHex: "#ABCDEF"},
		{Kind: ordinary.ChoiceNew, Name: "规划", ColorHex: "#654321"},
	}
	for i, choice := range choices {
		chain := 1
		if i >= 4 {
			chain = 2
		}
		request := ordinary.Request{ChainID: core.TaskChainID(testID(chain)), Choice: choice, InteractionID: ordinary.UUID(testID(100 + i))}
		ids := ordinary.PrepareIDs{PlanID: ordinary.UUID(testID(200 + i)), CategoryID: ordinary.CategoryID(testID(300 + i))}
		plan, err := ordinary.Prepare(ctx, state, request, ids)
		must(err)
		data, err := plan.Data()
		must(err)
		confirm, err := ordinary.ConfirmUser(plan, request.InteractionID)
		must(err)
		now := core.Instant(math.Float64bits(float64(i) / 10000))
		if i == 0 {
			now = core.Instant(math.Float64bits(math.Copysign(0, -1)))
		}
		commit := ordinary.CommitIDs{RecordID: ordinary.UUID(testID(400 + i)), NameVersionID: ordinary.UUID(testID(500 + i)), RemovalID: ordinary.UUID(testID(600 + i))}
		next, receipt, err := ordinary.Apply(ctx, state, plan, confirm, now, commit)
		must(err)
		step := ordinary.CommittedStep{Request: request, PrepareIDs: ids, Plan: data, DecisionID: request.InteractionID, Now: now, CommitIDs: commit, Receipt: receipt}
		steps = append(steps, step)
		state = next
		if i == 1 {
			retry, receipt2, err := ordinary.Apply(ctx, state, plan, confirm, core.Instant(math.Float64bits(math.NaN())), ordinary.CommitIDs{})
			must(err)
			if !reflect.DeepEqual(state, retry) || !reflect.DeepEqual(receipt, receipt2) {
				t.Fatal("retry changed facts")
			}
			step.Now = core.Instant(math.Float64bits(math.NaN()))
			step.CommitIDs = ordinary.CommitIDs{}
			steps = append(steps, step)
		}
	}
	proof, err := ordinary.Replay(ctx, steps)
	must(err)
	replayed, err := proof.State()
	must(err)
	if !reflect.DeepEqual(state, replayed) {
		t.Fatal("prepare/apply does not match causal replay")
	}
	must(ordinary.ValidateDecodedEntities(ctx, state))
	return ctx, state, steps
}
func ordinarySnapshot(t testing.TB) canonical.Value {
	t.Helper()
	ctx, state, _ := ordinaryFixture(t)
	s, err := ExportOrdinaryCategoryCandidate(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ImportOrdinaryCategoryCandidate(s, canonical.Limits{}); err != nil {
		t.Fatal("positive control", err)
	}
	return s
}
func ordinaryEnvelope(t testing.TB, s canonical.Value) []byte {
	t.Helper()
	b, err := canonical.Marshal(objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": s}))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestOrdinaryAdapterRoundTripAndProofSeparation(t *testing.T) {
	ctx, state, steps := ordinaryFixture(t)
	wire, err := ExportOrdinaryCategoryCandidate(ctx, state)
	if err != nil {
		t.Fatal(err)
	}
	before := ordinaryEnvelope(t, wire)
	candidate, err := DecodeOrdinaryCategoryCandidate(before, canonical.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	after, err := ExportOrdinaryCategoryCandidate(candidate.Context, candidate.State)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, ordinaryEnvelope(t, after)) {
		t.Fatal("wire bytes, optional fields, or bits changed")
	}
	if len(candidate.State.Categories) != 2 || candidate.State.Revision != 8 || len(candidate.State.RelationHistory) != 2 {
		t.Fatal("main sequence facts lost")
	}
	cat := candidate.State.Categories[ordinary.CategoryID(testID(300))]
	if cat.Name != "工程/\\\"e\u0301" || uint64(cat.CreatedAt) != math.Float64bits(math.Copysign(0, -1)) {
		t.Fatal("normalization or float conversion changed original bytes")
	}
	if _, ok := wire.Object["classifications"].Object["categories"].Array[1].Object["nameVersions"].Array[0].Object["validUntil"]; ok {
		t.Fatal("absent optional emitted")
	}
	if _, err = New(Options{}).Decode(before); !errors.Is(err, ErrDomainValidationRequired) {
		t.Fatal("adapter changed default codec admission", err)
	}
	if _, err = ordinary.Continue(ordinary.ReplayProof{}, steps); err == nil {
		t.Fatal("zero proof became authority")
	}
	// The imported state can be changed without changing the wire or context.
	cat.Name = "tampered"
	candidate.State.Categories[cat.ID] = cat
	if _, err = ExportOrdinaryCategoryCandidate(candidate.Context, candidate.State); err == nil {
		t.Fatal("export accepted mutable-state tampering")
	}
	if !bytes.Equal(before, ordinaryEnvelope(t, wire)) {
		t.Fatal("import/export mutated input")
	}
}

func TestOrdinaryAdapterUnknownFieldsAtEveryObject(t *testing.T) {
	good := ordinarySnapshot(t)
	var paths [][]any
	var visit func(canonical.Value, []any)
	visit = func(v canonical.Value, path []any) {
		if v.Kind == canonical.Object {
			paths = append(paths, append([]any{}, path...))
			for k, c := range v.Object {
				visit(c, append(append([]any{}, path...), k))
			}
		}
		if v.Kind == canonical.Array {
			for i, c := range v.Array {
				visit(c, append(append([]any{}, path...), i))
			}
		}
	}
	visit(good, nil)
	for i, path := range paths {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			v := clone(good)
			node := v
			for _, part := range path {
				switch p := part.(type) {
				case string:
					node = node.Object[p]
				case int:
					node = node.Array[p]
				}
			}
			node.Object["unrecognizedField"] = stringValue("reject")
			if _, err := ImportOrdinaryCategoryCandidate(v, canonical.Limits{}); err == nil {
				t.Fatal("unknown field was discarded")
			}
		})
	}
}

func TestOrdinaryAdapterRejectsScopeAndEntityMutations(t *testing.T) {
	good := ordinarySnapshot(t)
	c := func(s canonical.Value) canonical.Value { return s.Object["classifications"] }
	category := func(s canonical.Value) canonical.Value { return c(s).Object["categories"].Array[1] }
	record := func(s canonical.Value) canonical.Value { return c(s).Object["changeRecords"].Array[0] }
	current := func(s canonical.Value) canonical.Value {
		return c(s).Object["currentByChainID"].Array[3].Object["category"]
	}
	mutations := map[string]func(canonical.Value){
		"state-version":     func(s canonical.Value) { c(s).Object["formatVersion"] = numberValue("2") },
		"claimed-revision":  func(s canonical.Value) { c(s).Object["revision"] = numberValue("18446744073709551615") },
		"unknown-algorithm": func(s canonical.Value) { category(s).Object["canonicalKeyVersion"] = stringValue("unknown") },
		"unknown-version-algorithm": func(s canonical.Value) {
			category(s).Object["nameVersions"].Array[0].Object["canonicalKeyVersion"] = stringValue("unknown")
		},
		"bad-key":  func(s canonical.Value) { category(s).Object["canonicalKey"] = stringValue("changed") },
		"bad-name": func(s canonical.Value) { category(s).Object["name"] = stringValue(" engineering ") },
		"name-version-mismatch": func(s canonical.Value) {
			category(s).Object["nameVersions"].Array[0].Object["name"] = stringValue("different")
		},
		"closed-version": func(s canonical.Value) {
			category(s).Object["nameVersions"].Array[0].Object["validUntil"] = numberValue("0")
		},
		"extra-name-version": func(s canonical.Value) {
			item := category(s)
			a := item.Object["nameVersions"]
			a.Array = append(a.Array, clone(a.Array[0]))
			item.Object["nameVersions"] = a
		},
		"pending-ai":          func(s canonical.Value) { category(s).Object["presentationApproval"] = stringValue("pendingAIReview") },
		"archived":            func(s canonical.Value) { category(s).Object["lifecycle"] = stringValue("archived") },
		"unknown-lifecycle":   func(s canonical.Value) { category(s).Object["lifecycle"] = stringValue("future") },
		"missing-entity-id":   func(s canonical.Value) { delete(category(s).Object, "id") },
		"category-id-wrapper": func(s canonical.Value) { category(s).Object["id"] = stringValue(testID(300)) },
		"lowercase-id": func(s canonical.Value) {
			s.Object["chains"].Array[0].Object["id"].Object["rawValue"] = stringValue("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
		},
		"unknown-task-enum": func(s canonical.Value) { s.Object["chains"].Array[0].Object["state"] = stringValue("future") },
		"invalid-graph": func(s canonical.Value) {
			s.Object["definitions"].Array[0].Object["chainID"].Object["rawValue"] = stringValue(testID(999))
		},
		"cycle-membership": func(s canonical.Value) {
			s.Object["chains"].Array[0].Object["cycleMembership"] = objectValue(map[string]canonical.Value{"seriesID": objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(999))}), "occurrenceDate": objectValue(map[string]canonical.Value{"year": numberValue("2026"), "month": numberValue("11"), "day": numberValue("7")})})
		},
		"preferences": func(s canonical.Value) { s.Object["preferences"].Object["language"] = stringValue("english") },
		"preference-bits": func(s canonical.Value) {
			s.Object["preferences"].Object["themeLanguageUpdatedAt"] = numberValue("9223372036854775808")
		},
		"current-chain-reference": func(s canonical.Value) {
			c(s).Object["currentByChainID"].Array[0].Object["rawValue"] = stringValue(testID(999))
		},
		"relation-ai": func(s canonical.Value) {
			current(s).Object["source"] = tagged("automaticAI", map[string]canonical.Value{"jobID": stringValue(testID(999)), "generation": numberValue("1")})
		},
		"record-source": func(s canonical.Value) {
			record(s).Object["source"] = tagged("inherited", map[string]canonical.Value{"fromChainID": objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(1))})})
		},
		"unknown-source":       func(s canonical.Value) { record(s).Object["source"] = tagged("future", map[string]canonical.Value{}) },
		"record-digest":        func(s canonical.Value) { record(s).Object["integrityDigest"] = stringValue(strings.Repeat("0", 64)) },
		"plan-digest-shape":    func(s canonical.Value) { record(s).Object["planDigest"] = stringValue("short") },
		"decision-missing":     func(s canonical.Value) { delete(record(s).Object, "decisionID") },
		"record-empty-changes": func(s canonical.Value) { record(s).Object["changes"] = ordinaryArray() },
		"record-multi-changes": func(s canonical.Value) {
			r := record(s)
			a := r.Object["changes"]
			a.Array = append(a.Array, clone(a.Array[0]))
			r.Object["changes"] = a
		},
		"management-create": func(s canonical.Value) {
			record(s).Object["changes"] = ordinaryArray(tagged("create", map[string]canonical.Value{"kind": stringValue("category"), "itemID": stringValue(testID(300)), "name": stringValue("工程"), "colorHex": stringValue("#123456")}))
		},
		"preview-alias": func(s canonical.Value) {
			record(s).Object["changes"].Array[0].Object["setCurrent"].Object["after"].Object["category"].Object["resolution"] = stringValue("historicalAlias")
		},
		"preview-kind": func(s canonical.Value) {
			record(s).Object["changes"].Array[0].Object["setCurrent"].Object["after"].Object["category"].Object["kind"] = stringValue("label")
		},
		"preview-label": func(s canonical.Value) {
			p := record(s).Object["changes"].Array[0].Object["setCurrent"].Object["after"]
			p.Object["labels"] = ordinaryArray(clone(p.Object["category"]))
		},
		"notice-alias": func(s canonical.Value) {
			c(s).Object["changeRecords"].Array[6].Object["notices"].Array[0].Object["existingItemReused"].Object["matchedHistoricalAlias"] = canonical.Value{Kind: canonical.Boolean, Bool: true}
		},
		"notice-label": func(s canonical.Value) {
			record(s).Object["notices"] = ordinaryArray(tagged("duplicateLabelCollapsed", map[string]canonical.Value{"name": stringValue("x")}))
		},
		"receipt-bind": func(s canonical.Value) {
			c(s).Object["committedReceiptsByInteractionID"].Array[1].Object["changeRecordID"] = stringValue(testID(999))
		},
		"missing-receipt": func(s canonical.Value) {
			a := c(s).Object["committedReceiptsByInteractionID"]
			a.Array = a.Array[2:]
			c(s).Object["committedReceiptsByInteractionID"] = a
		},
		"missing-removal": func(s canonical.Value) {
			a := c(s).Object["relationHistory"]
			a.Array = a.Array[1:]
			c(s).Object["relationHistory"] = a
		},
		"removal-source": func(s canonical.Value) {
			c(s).Object["relationHistory"].Array[0].Object["removedBySource"] = tagged("deterministicDomainAction", map[string]canonical.Value{"reason": stringValue("x")})
		},
		"removal-chain": func(s canonical.Value) {
			c(s).Object["relationHistory"].Array[0].Object["chainID"].Object["rawValue"] = stringValue(testID(999))
		},
		"invalid-time-bits": func(s canonical.Value) { record(s).Object["committedAt"] = numberValue("9218868437227405312") },
		"float-instant":     func(s canonical.Value) { record(s).Object["committedAt"] = numberValue("0.1") },
		"optional-null": func(s canonical.Value) {
			category(s).Object["nameVersions"].Array[0].Object["validUntil"] = canonical.Value{Kind: canonical.Null}
		},
		"long-nonstarter": func(s canonical.Value) {
			category(s).Object["name"] = stringValue("a" + strings.Repeat("\u0301", 20_000))
		},
	}
	// Structurally valid unsupported sections are rejected as scope, before they
	// can vanish into a smaller typed State. Build their fields from the schema.
	for _, key := range ordinaryEmptySections {
		key := key
		mutations["scope-"+key] = func(s canonical.Value) {
			typ := objects["Classifications"][key]
			c(s).Object[key] = ordinaryNonemptySchemaValue(typ)
		}
	}
	for _, key := range []string{"ideas", "taskCycleSeries"} {
		key := key
		mutations["scope-"+key] = func(s canonical.Value) { s.Object[key] = ordinaryNonemptySchemaValue(objects["Snapshot"][key]) }
	}
	mutations["current-label"] = func(s canonical.Value) {
		c(s).Object["currentByChainID"].Array[1].Object["labels"] = ordinaryNonemptySchemaValue("[]LabelRelation")
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			input := clone(good)
			mutate(input)
			if _, err := ImportOrdinaryCategoryCandidate(input, canonical.Limits{}); err == nil {
				t.Fatal("invalid import accepted")
			}
		})
	}
}

// Schema-shaped synthetic unsupported facts, not an alternate domain generator.
// The positive control is always the real model sequence above.
func ordinaryNonemptySchemaValue(typ string) canonical.Value {
	if strings.HasPrefix(typ, "[]") {
		return ordinaryArray(ordinarySchemaValue(typ[2:]))
	}
	if strings.HasPrefix(typ, "pairs:") {
		p := strings.SplitN(typ[6:], ":", 2)
		return ordinaryArray(ordinarySchemaValue(p[0]), ordinarySchemaValue(p[1]))
	}
	return ordinarySchemaValue(typ)
}
func ordinarySchemaValue(typ string) canonical.Value {
	if alias, ok := aliases[typ]; ok {
		return ordinarySchemaValue(alias)
	}
	if strings.HasPrefix(typ, "[]") || strings.HasPrefix(typ, "pairs:") {
		return ordinaryArray()
	}
	if fs, ok := objects[typ]; ok {
		out := map[string]canonical.Value{}
		for key, t := range fs {
			if strings.HasSuffix(key, "?") {
				continue
			}
			out[key] = ordinarySchemaValue(t)
		}
		if typ == "LocalDate" {
			out["year"], out["month"], out["day"] = numberValue("2026"), numberValue("11"), numberValue("7")
		}
		return objectValue(out)
	}
	if cases, ok := unions[typ]; ok {
		key := ""
		for k := range cases {
			if key == "" || k < key {
				key = k
			}
		}
		out := map[string]canonical.Value{}
		for k, t := range cases[key] {
			out[k] = ordinarySchemaValue(t)
		}
		return tagged(key, out)
	}
	if strings.HasPrefix(typ, "enum:") {
		return stringValue(strings.Split(typ[5:], ",")[0])
	}
	if strings.HasPrefix(typ, "enumint:") {
		return numberValue(strings.Split(typ[8:], ",")[0])
	}
	switch typ {
	case "uuid":
		return stringValue(testID(9000))
	case "string":
		return stringValue("synthetic")
	case "bool":
		return canonical.Value{Kind: canonical.Boolean}
	default:
		return numberValue("0")
	}
}

func TestOrdinaryAdapterUnknownEnumsAtEveryOccurrence(t *testing.T) {
	good := ordinarySnapshot(t)
	type mutation struct {
		path   []any
		union  bool
		number bool
	}
	var mutations []mutation
	var visit func(canonical.Value, string, []any)
	visit = func(v canonical.Value, typ string, path []any) {
		if alias, ok := aliases[typ]; ok {
			visit(v, alias, path)
			return
		}
		if strings.HasPrefix(typ, "enum:") || strings.HasPrefix(typ, "enumint:") {
			mutations = append(mutations, mutation{append([]any{}, path...), false, strings.HasPrefix(typ, "enumint:")})
			return
		}
		if strings.HasPrefix(typ, "[]") {
			for i, item := range v.Array {
				visit(item, typ[2:], append(append([]any{}, path...), i))
			}
			return
		}
		if strings.HasPrefix(typ, "pairs:") {
			parts := strings.SplitN(typ[6:], ":", 2)
			for i, item := range v.Array {
				visit(item, parts[i%2], append(append([]any{}, path...), i))
			}
			return
		}
		if fs, ok := objects[typ]; ok {
			for k, t := range fs {
				k = strings.TrimSuffix(k, "?")
				if item, exists := v.Object[k]; exists {
					visit(item, t, append(append([]any{}, path...), k))
				}
			}
			return
		}
		if cases, ok := unions[typ]; ok {
			mutations = append(mutations, mutation{append([]any{}, path...), true, false})
			for tag, body := range v.Object {
				for k, t := range cases[tag] {
					visit(body.Object[k], t, append(append(append([]any{}, path...), tag), k))
				}
			}
		}
	}
	visit(good, "Snapshot", nil)
	for i, m := range mutations {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			input := clone(good)
			node := input
			for _, part := range m.path[:len(m.path)-1] {
				switch p := part.(type) {
				case string:
					node = node.Object[p]
				case int:
					node = node.Array[p]
				}
			}
			replacement := stringValue("unrecognized-enum")
			if m.union {
				replacement = tagged("unrecognizedEnum", map[string]canonical.Value{})
			}
			if m.number {
				replacement = numberValue("999")
			}
			switch p := m.path[len(m.path)-1].(type) {
			case string:
				node.Object[p] = replacement
			case int:
				node.Array[p] = replacement
			}
			if _, err := ImportOrdinaryCategoryCandidate(input, canonical.Limits{}); err == nil {
				t.Fatal("unknown enum projected away")
			}
		})
	}
}

func TestOrdinaryAdapterDoesNotRepairIdentitySpelling(t *testing.T) {
	good := ordinarySnapshot(t)
	var lower func(canonical.Value) canonical.Value
	lower = func(v canonical.Value) canonical.Value {
		if v.Kind == canonical.String && validUUID(v.String) {
			v.String = strings.ToLower(v.String)
		}
		for k, item := range v.Object {
			v.Object[k] = lower(item)
		}
		for i, item := range v.Array {
			v.Array[i] = lower(item)
		}
		return v
	}
	input := lower(clone(good))
	if bytes.Equal(ordinaryEnvelope(t, input), ordinaryEnvelope(t, good)) {
		t.Fatal("identity mutation did not change control")
	}
	if _, err := ImportOrdinaryCategoryCandidate(input, canonical.Limits{}); err == nil {
		t.Fatal("noncanonical identity bytes were silently uppercased")
	}
}
