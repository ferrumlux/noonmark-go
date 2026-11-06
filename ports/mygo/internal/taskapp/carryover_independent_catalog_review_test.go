// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"reflect"
	"testing"
)

func TestIndependentCarryoverRefusesValidRichCatalog(t *testing.T) {
	for _, cont := range []bool{false, true} {
		for _, kind := range []string{"categories", "labels"} {
			t.Run(map[bool]string{false: "defer/", true: "continue/"}[cont]+kind, func(t *testing.T) {
				s, _, tr := prepareCarryover(t, cont)
				bad, e := inspectEmptyHistory(s.snapshot)
				must(t, e)
				key := historyObject(map[string]canonical.Value{"rawValue": historyString("00000000-0000-4000-8000-000000000099")})
				item := historyObject(map[string]canonical.Value{"id": key, "name": historyString("Synthetic"), "canonicalKey": historyString("synthetic"), "canonicalKeyVersion": historyString("synthetic-test"), "colorHex": historyString("#123456"), "lifecycle": historyString("active"), "nameVersions": {Kind: canonical.Array}, "createdAt": historyNumber(0), "updatedAt": historyNumber(0)})
				if kind == "categories" {
					item.Object["presentationApproval"] = historyString("userApproved")
				}
				bad.Object["classifications"].Object[kind] = canonical.Value{Kind: canonical.Array, Array: []canonical.Value{key, item}}
				if _, e := inspectEmptyHistory(bad); e != nil {
					t.Fatalf("invalid fixture, not scope test: %v", e)
				}
				s.snapshot = bad
				before := s.Snapshot()
				revision := s.disk.revision
				reserved := false
				s.disk.fault = func(phase string) error { reserved = true; return nil }
				target, _ := s.Today().AddDays(1)
				id, e := callCarryover(s, tr, target, cont)
				if e == nil || id != "" || reserved || s.disk.revision != revision || !reflect.DeepEqual(before, s.Snapshot()) || !historyEqual(bad, s.snapshot) {
					t.Fatal("rich catalog reached persistence or publication")
				}
			})
		}
	}
}
