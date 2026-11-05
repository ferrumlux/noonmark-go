// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
)

func catalogFixture(t *testing.T) canonical.Value {
	t.Helper()
	s, _ := fixture(t)
	c := s.Object["classifications"]
	category := parse(t, fmt.Sprintf(`{"id":{"rawValue":%q},"name":"工作","canonicalKey":"工作","canonicalKeyVersion":"synthetic-only","colorHex":"#123456","presentationApproval":"userApproved","lifecycle":"active","nameVersions":[{"id":%q,"name":"工作","canonicalKey":"工作","canonicalKeyVersion":"synthetic-only","validFrom":%d}],"createdAt":%d,"updatedAt":%d}`, testID(980), testID(981), testTime(100), testTime(100), testTime(100)))
	label := clone(category)
	label.Object["id"] = objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(982))})
	delete(label.Object, "presentationApproval")
	c.Object["categories"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{clone(category.Object["id"]), category}}
	c.Object["labels"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{clone(label.Object["id"]), label}}
	current := parse(t, fmt.Sprintf(`{"category":{"categoryID":{"rawValue":%q},"source":{"userDirect":{}},"createdAt":%d,"updatedAt":%d,"revision":1},"labels":[{"labelID":{"rawValue":%q},"source":{"automaticAI":{"jobID":%q,"generation":1}},"createdAt":%d,"updatedAt":%d,"revision":1}]}`, testID(980), testTime(100), testTime(100), testID(982), testID(983), testTime(100), testTime(100)))
	c.Object["currentByChainID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{clone(s.Object["chains"].Array[0].Object["id"]), current}}
	s.Object["ideas"].Array[0].Object["categoryID"] = clone(category.Object["id"])
	s.Object["ideas"].Array[0].Object["labelIDs"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{clone(label.Object["id"])}}
	s.Object["classifications"] = c
	return s
}
func TestNonemptyWireClassificationIsLosslessButRequiresAuditAuthority(t *testing.T) {
	snapshot := catalogFixture(t)
	b := rawEnvelope(t, snapshot)
	inspected, e := New(Options{}).Inspect(b)
	if e != nil || !inspected.RequiresClassificationAudit {
		t.Fatal(e)
	}
	if _, e = New(Options{}).Decode(b); !errors.Is(e, ErrDomainValidationRequired) {
		t.Fatal(e)
	}
	// Exact synthetic allowlist exercises the transport seam ONLY. This deliberately
	// incomplete audit fixture is not evidence of Swift audit acceptance.
	calls := 0
	codec := New(Options{ClassificationAuditValidator: func(s canonical.Value) error {
		calls++
		if !reflect.DeepEqual(s, snapshot) {
			return errors.New("not the registered synthetic fixture")
		}
		return nil
	}})
	doc, e := codec.Decode(b)
	if e != nil {
		t.Fatal(e)
	}
	out, e := codec.Encode(doc.Snapshot())
	if e != nil || string(out) != string(b) || calls != 2 {
		t.Fatal("wire classification lost", e)
	}
}
func TestAlternatingDictionariesRejectDuplicateKeysAndWrongShapes(t *testing.T) {
	original := catalogFixture(t)
	codec := New(Options{})
	mutate := []func(canonical.Value){
		func(v canonical.Value) {
			c := v.Object["classifications"]
			a := c.Object["categories"]
			a.Array = append(a.Array, a.Array...)
			c.Object["categories"] = a
		},
		func(v canonical.Value) {
			c := v.Object["classifications"]
			a := c.Object["categories"]
			a.Array = a.Array[:1]
			c.Object["categories"] = a
		},
		func(v canonical.Value) {
			v.Object["classifications"].Object["categories"] = objectValue(map[string]canonical.Value{testID(980): v.Object["classifications"].Object["categories"].Array[1]})
		},
		func(v canonical.Value) {
			v.Object["classifications"].Object["categories"].Array[0].Object["rawValue"] = stringValue(testID(999))
		},
		func(v canonical.Value) {
			v.Object["classifications"].Object["currentByChainID"].Array[0].Object["rawValue"] = stringValue(testID(999))
		},
		func(v canonical.Value) {
			v.Object["classifications"].Object["currentByChainID"].Array[1].Object["category"].Object["categoryID"].Object["rawValue"] = stringValue(testID(999))
		},
	}
	for _, change := range mutate {
		v := clone(original)
		change(v)
		if _, e := codec.Inspect(rawEnvelope(t, v)); e == nil {
			t.Fatal("invalid dictionary accepted")
		}
	}
}
func TestSourceReferencesAndUnionShapesAreValidated(t *testing.T) {
	original := catalogFixture(t)
	codec := New(Options{})
	for _, source := range []canonical.Value{
		tagged("inherited", map[string]canonical.Value{"fromChainID": objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(999))})}),
		tagged("automaticAI", map[string]canonical.Value{"jobID": stringValue(testID(999)), "generation": numberValue("0")}),
		tagged("userDirect", map[string]canonical.Value{"jobID": stringValue(testID(999))}),
	} {
		v := clone(original)
		v.Object["classifications"].Object["currentByChainID"].Array[1].Object["category"].Object["source"] = source
		if _, e := codec.Inspect(rawEnvelope(t, v)); e == nil {
			t.Fatal("invalid source accepted")
		}
	}
}
func TestMergeAndDeletionCannotOrphanCatalogFacts(t *testing.T) {
	original := catalogFixture(t)
	codec := New(Options{})
	// Source marked merged but no merge record.
	v := clone(original)
	v.Object["classifications"].Object["categories"].Array[1].Object["lifecycle"] = stringValue("merged")
	if _, e := codec.Inspect(rawEnvelope(t, v)); e == nil {
		t.Fatal("merged lifecycle without fact accepted")
	}
	// Tombstone identity must be absent from the live catalog.
	tombstone := parse(t, fmt.Sprintf(`{"id":%q,"itemID":{"rawValue":%q},"deletedAt":%d,"revision":1,"changeRecordID":%q}`, testID(990), testID(980), testTime(100), testID(991)))
	v = clone(original)
	v.Object["classifications"].Object["categoryDeletionTombstones"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{clone(tombstone.Object["itemID"]), tombstone}}
	if _, e := codec.Inspect(rawEnvelope(t, v)); e == nil {
		t.Fatal("live tombstone accepted")
	}
}
func TestAllDeclaredWireSchemasHaveSyntheticShapeFixtures(t *testing.T) {
	// Shape-only cases exercise every declared optional field and enum case. They
	// do not bypass Inspect/Decode or imply that arbitrary domain facts are valid.
	var sample func(string) canonical.Value
	sample = func(typ string) canonical.Value {
		if alias, ok := aliases[typ]; ok {
			return sample(alias)
		}
		if strings.HasPrefix(typ, "[]") {
			return canonical.Value{Kind: canonical.Array, Array: []canonical.Value{sample(typ[2:])}}
		}
		if strings.HasPrefix(typ, "pairs:") {
			p := strings.SplitN(typ[6:], ":", 2)
			return canonical.Value{Kind: canonical.Array, Array: []canonical.Value{sample(p[0]), sample(p[1])}}
		}
		if fs, ok := objects[typ]; ok {
			m := map[string]canonical.Value{}
			for k, t := range fs {
				m[strings.TrimSuffix(k, "?")] = sample(t)
			}
			if typ == "LocalDate" {
				m["year"] = numberValue("2026")
				m["month"] = numberValue("7")
				m["day"] = numberValue("20")
			}
			return objectValue(m)
		}
		if cases, ok := unions[typ]; ok {
			for name, fs := range cases {
				m := map[string]canonical.Value{}
				for k, t := range fs {
					m[k] = sample(t)
				}
				return tagged(name, m)
			}
		}
		if strings.HasPrefix(typ, "enum:") {
			return stringValue(strings.Split(typ[5:], ",")[0])
		}
		if strings.HasPrefix(typ, "enumint:") {
			return numberValue(strings.Split(typ[8:], ",")[0])
		}
		switch typ {
		case "uuid":
			return stringValue(testID(1))
		case "string":
			return stringValue("fixture")
		case "int", "uint", "stamp":
			return numberValue("1")
		case "bool":
			return canonical.Value{Kind: canonical.Boolean}
		}
		panic(typ)
	}
	for typ := range objects {
		v := sample(typ)
		if e := validateType(v, typ, typ); e != nil {
			t.Fatal(e)
		}
	}
	for typ, cases := range unions {
		for name, fs := range cases {
			m := map[string]canonical.Value{}
			for k, t := range fs {
				m[k] = sample(t)
			}
			if e := validateType(tagged(name, m), typ, typ); e != nil {
				t.Fatal(e)
			}
		}
	}
}

func TestDictionaryUUIDCaseVariantsAreDuplicateIdentities(t *testing.T) {
	v := catalogFixture(t)
	catalog := v.Object["classifications"].Object["categories"]
	lowerKey := clone(catalog.Array[0])
	lowerKey.Object["rawValue"] = stringValue(strings.ToLower(lowerKey.Object["rawValue"].String))
	catalog.Array = append(catalog.Array, lowerKey, clone(catalog.Array[1]))
	v.Object["classifications"].Object["categories"] = catalog
	if _, e := New(Options{}).Inspect(rawEnvelope(t, v)); !errors.Is(e, ErrDuplicate) {
		t.Fatal(e)
	}
}

func TestMergeGraphInspectionRejectsCyclesAndInactiveEndpoints(t *testing.T) {
	snapshot := catalogFixture(t)
	c := snapshot.Object["classifications"]
	template := clone(c.Object["categories"].Array[1])
	categories := canonical.Value{Kind: canonical.Array, Array: []canonical.Value{}}
	merges := canonical.Value{Kind: canonical.Array, Array: []canonical.Value{}}
	for i := 0; i < 32; i++ {
		item := clone(template)
		id := objectValue(map[string]canonical.Value{"rawValue": stringValue(testID(1000 + i))})
		item.Object["id"] = id
		if i < 31 {
			item.Object["lifecycle"] = stringValue("merged")
		}
		categories.Array = append(categories.Array, id, item)
		if i < 31 {
			m := parse(t, fmt.Sprintf(`{"id":%q,"sourceID":{"rawValue":%q},"targetID":{"rawValue":%q},"mergedAt":%d,"revision":1,"changeRecordID":%q}`, testID(1100+i), testID(1000+i), testID(1001+i), testTime(100), testID(1200+i)))
			merges.Array = append(merges.Array, id, m)
		}
	}
	c.Object["categories"] = categories
	c.Object["categoryMerges"] = merges
	c.Object["currentByChainID"] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{}}
	snapshot.Object["classifications"] = c
	delete(snapshot.Object["ideas"].Array[0].Object, "categoryID")
	if _, e := New(Options{}).Inspect(rawEnvelope(t, snapshot)); e != nil {
		t.Fatal(e)
	}
	cyclic := clone(snapshot)
	cyclic.Object["classifications"].Object["categoryMerges"].Array[1].Object["targetID"] = clone(cyclic.Object["classifications"].Object["categoryMerges"].Array[0])
	if _, e := New(Options{}).Inspect(rawEnvelope(t, cyclic)); e == nil {
		t.Fatal("self cycle accepted")
	}
	cycle := clone(snapshot)
	cycle.Object["classifications"].Object["categoryMerges"].Array[3].Object["targetID"] = clone(cycle.Object["classifications"].Object["categoryMerges"].Array[0])
	if _, e := New(Options{}).Inspect(rawEnvelope(t, cycle)); e == nil {
		t.Fatal("multi-node cycle accepted")
	}
	inactive := clone(snapshot)
	inactive.Object["classifications"].Object["categories"].Array[63].Object["lifecycle"] = stringValue("archived")
	if _, e := New(Options{}).Inspect(rawEnvelope(t, inactive)); e == nil {
		t.Fatal("inactive merge endpoint accepted")
	}
}
