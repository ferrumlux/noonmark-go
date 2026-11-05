// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
)

func wireID(v canonical.Value) string { return strings.ToUpper(v.Object["rawValue"].String) }
func pairs(v canonical.Value) map[string]canonical.Value {
	out := map[string]canonical.Value{}
	for i := 0; i < len(v.Array); i += 2 {
		k := strings.ToUpper(v.Array[i].String)
		if v.Array[i].Kind == canonical.Object {
			k = wireID(v.Array[i])
		}
		out[k] = v.Array[i+1]
	}
	return out
}

type catalogSets struct{ categories, labels map[string]bool }

func catalogIDs(c canonical.Value) catalogSets {
	out := catalogSets{map[string]bool{}, map[string]bool{}}
	for k := range pairs(c.Object["categories"]) {
		out.categories[k] = true
	}
	for k := range pairs(c.Object["labels"]) {
		out.labels[k] = true
	}
	return out
}
func missing(path string) error   { return fmt.Errorf("%w: %s", ErrMissingReference, path) }
func duplicate(path string) error { return fmt.Errorf("%w: %s", ErrDuplicate, path) }
func requireUniqueIDs(values []canonical.Value, field, path string, wrapped bool) error {
	seen := map[string]bool{}
	for _, v := range values {
		x := v.Object[field]
		id := strings.ToUpper(x.String)
		if wrapped {
			id = wireID(x)
		}
		if seen[id] {
			return duplicate(path)
		}
		seen[id] = true
	}
	return nil
}
func validateClassificationReferences(snapshot canonical.Value) error {
	c := snapshot.Object["classifications"]
	categories := pairs(c.Object["categories"])
	labels := pairs(c.Object["labels"])
	chains := map[string]bool{}
	traces := map[string]bool{}
	for _, v := range snapshot.Object["chains"].Array {
		chains[wireID(v.Object["id"])] = true
	}
	for _, v := range snapshot.Object["traces"].Array {
		traces[wireID(v.Object["id"])] = true
	}
	for _, kind := range []string{"categories", "labels"} {
		for key, item := range pairs(c.Object[kind]) {
			if key != wireID(item.Object["id"]) {
				return missing(kind + " identity/key mismatch")
			}
		}
	}
	for chain, current := range pairs(c.Object["currentByChainID"]) {
		if !chains[chain] {
			return missing("current classification chain")
		}
		if category, ok := current.Object["category"]; ok {
			if _, exists := categories[wireID(category.Object["categoryID"])]; !exists {
				return missing("current category")
			}
		}
		if e := requireUniqueIDs(current.Object["labels"].Array, "labelID", "current label", true); e != nil {
			return e
		}
		for _, label := range current.Object["labels"].Array {
			if _, ok := labels[wireID(label.Object["labelID"])]; !ok {
				return missing("current label")
			}
		}
	}
	if e := requireUniqueIDs(c.Object["relationHistory"].Array, "id", "relation history", false); e != nil {
		return e
	}
	for _, h := range c.Object["relationHistory"].Array {
		if !chains[wireID(h.Object["chainID"])] {
			return missing("relation history chain")
		}
		if unsigned(h.Object["createdRevision"]) > unsigned(h.Object["removedRevision"]) || stampSeconds(h.Object["createdAt"]) > stampSeconds(h.Object["removedAt"]) {
			return malformed("classifications.relationHistory", "backwards history")
		}
	}
	snapshots := pairs(c.Object["snapshotsByTraceID"])
	streams := pairs(c.Object["snapshotEventsByTraceID"])
	eventsSeen := map[string]bool{}
	checkSnapshot := func(key string, event canonical.Value) error {
		if !traces[key] || wireID(event.Object["traceID"]) != key {
			return missing("classification snapshot trace")
		}
		if category, ok := event.Object["category"]; ok {
			if _, exists := categories[wireID(category.Object["id"])]; !exists {
				return missing("historical category")
			}
		}
		if e := requireUniqueIDs(event.Object["labels"].Array, "id", "historical labels", true); e != nil {
			return e
		}
		for _, label := range event.Object["labels"].Array {
			if _, ok := labels[wireID(label.Object["id"])]; !ok {
				return missing("historical label")
			}
		}
		return nil
	}
	for key, event := range snapshots {
		if e := checkSnapshot(key, event); e != nil {
			return e
		}
	}
	for key, stream := range streams {
		if !traces[key] || len(stream.Array) == 0 {
			return missing("empty or orphan classification event stream")
		}
		latest, exists := snapshots[key]
		if !exists || !reflect.DeepEqual(latest, stream.Array[len(stream.Array)-1]) {
			return missing("classification latest snapshot/event mismatch")
		}
		for _, event := range stream.Array {
			if e := checkSnapshot(key, event); e != nil {
				return e
			}
			id := strings.ToUpper(event.Object["id"].String)
			if eventsSeen[id] {
				return duplicate("classification event id")
			}
			eventsSeen[id] = true
		}
	}
	records := map[string]canonical.Value{}
	for _, r := range c.Object["changeRecords"].Array {
		id := strings.ToUpper(r.Object["id"].String)
		if _, ok := records[id]; ok {
			return duplicate("classification change record")
		}
		records[id] = r
	}
	for _, kind := range []string{"category", "label"} {
		live := categories
		if kind == "label" {
			live = labels
		}
		tombstones := pairs(c.Object[kind+"DeletionTombstones"])
		tombstoneIDs := map[string]bool{}
		for key, t := range tombstones {
			if key != wireID(t.Object["itemID"]) {
				return missing("tombstone key")
			}
			if _, exists := live[key]; exists {
				return malformed("classifications", "deleted classification identity still live")
			}
			id := strings.ToUpper(t.Object["id"].String)
			if tombstoneIDs[id] {
				return duplicate("tombstone id")
			}
			tombstoneIDs[id] = true
			if _, ok := records[strings.ToUpper(t.Object["changeRecordID"].String)]; !ok {
				return missing("tombstone change record")
			}
		}
		merges := pairs(c.Object[kind+"Merges"])
		for key, m := range merges {
			source, ok := live[key]
			if !ok || key != wireID(m.Object["sourceID"]) || source.Object["lifecycle"].String != "merged" {
				return missing("merge source/lifecycle")
			}
			target := wireID(m.Object["targetID"])
			if target == key {
				return malformed("classifications", "self merge")
			}
			if _, ok := live[target]; !ok {
				return missing("merge target")
			}
		}
		for key, item := range live {
			_, merged := merges[key]
			if (item.Object["lifecycle"].String == "merged") != merged {
				return missing("merged lifecycle fact")
			}
		}

		resolved := map[string]bool{}
		for key := range merges {
			if resolved[key] {
				continue
			}
			seen := map[string]bool{}
			var path []string
			cursor := key
			for {
				if resolved[cursor] {
					break
				}
				m, ok := merges[cursor]
				if !ok {
					if live[cursor].Object["lifecycle"].String != "active" {
						return malformed("classifications", "merge must end at active item")
					}
					break
				}
				if seen[cursor] {
					return malformed("classifications", "merge cycle")
				}
				seen[cursor] = true
				path = append(path, cursor)
				cursor = wireID(m.Object["targetID"])
			}
			for _, id := range path {
				resolved[id] = true
			}
		}

	}

	for _, current := range pairs(c.Object["currentByChainID"]) {
		if category, ok := current.Object["category"]; ok {
			if e := validateSourceReferences(category.Object["source"], chains); e != nil {
				return e
			}
		}
		for _, label := range current.Object["labels"].Array {
			if e := validateSourceReferences(label.Object["source"], chains); e != nil {
				return e
			}
		}
	}
	for _, h := range c.Object["relationHistory"].Array {
		for _, key := range []string{"originSource", "removedBySource"} {
			if e := validateSourceReferences(h.Object[key], chains); e != nil {
				return e
			}
		}
	}
	for _, r := range records {
		if e := validateSourceReferences(r.Object["source"], chains); e != nil {
			return e
		}
		for _, change := range r.Object["changes"].Array {
			if v, ok := change.Object["setCurrent"]; ok && !chains[wireID(v.Object["chainID"])] {
				return missing("classification audit chain")
			}
			if v, ok := change.Object["merge"]; ok {
				impact := v.Object["impact"]
				for _, id := range impact.Object["currentChainIDs"].Array {
					if !chains[wireID(id)] {
						return missing("merge impact chain")
					}
				}
				for _, id := range impact.Object["historicalTraceIDs"].Array {
					if !traces[wireID(id)] {
						return missing("merge impact trace")
					}
				}
				for _, id := range impact.Object["historicalEventIDs"].Array {
					if !eventsSeen[strings.ToUpper(id.String)] {
						return missing("merge impact event")
					}
				}
			}
		}
	}
	return nil
}
func validateSourceReferences(source canonical.Value, chains map[string]bool) error {
	if v, ok := source.Object["automaticAI"]; ok && signed(v.Object["generation"]) <= 0 {
		return malformed("classifications.source", "nonpositive generation")
	}
	if v, ok := source.Object["inherited"]; ok && !chains[wireID(v.Object["fromChainID"])] {
		return missing("classification source chain")
	}
	return nil
}
func stampSeconds(v canonical.Value) float64 { return math.Float64frombits(unsigned(v)) }
func validateIdeas(ideas canonical.Value, catalogs catalogSets) error {
	if e := requireUniqueIDs(ideas.Array, "id", "idea identity", true); e != nil {
		return e
	}
	for _, idea := range ideas.Array {
		o := idea.Object
		created, updated := stampSeconds(o["createdAt"]), stampSeconds(o["updatedAt"])
		if updated < created {
			return malformed("ideas", "backwards clock")
		}
		seen := map[string]bool{}
		for _, label := range o["labelIDs"].Array {
			id := wireID(label)
			if seen[id] {
				return duplicate("idea label")
			}
			seen[id] = true
		}
		pinned, hasPin := o["pinnedAt"]
		if hasPin && (stampSeconds(pinned) < created || stampSeconds(pinned) > updated) {
			return malformed("ideas", "invalid pin clock")
		}
		deleted, isDeleted := o["deletedAt"]
		if isDeleted {
			_, hasCategory := o["categoryID"]
			if hasPin || hasCategory || len(seen) > 0 || o["body"].String != "" || stampSeconds(deleted) < created || stampSeconds(deleted) != updated {
				return malformed("ideas", "invalid tombstone")
			}
		} else {
			body := o["body"].String
			if body == "" || strings.TrimSpace(body) != body {
				return malformed("ideas", "invalid active body")
			}
			if category, ok := o["categoryID"]; ok && !catalogs.categories[wireID(category)] {
				return missing("idea category")
			}
			for label := range seen {
				if !catalogs.labels[label] {
					return missing("idea label")
				}
			}
		}
	}
	return nil
}

// Bound inherited revision-ancestry validation before adapting untrusted input.
func validateWorkBudget(snapshot canonical.Value) error {
	total := 0
	for _, p := range snapshot.Object["taskCycleSeries"].Array {
		n := len(p.Object["planRevisions"].Array) + len(p.Object["classificationRevisions"].Array)
		if n > MaximumRevisionsPerSeries || total > MaximumTotalRevisions-n {
			return ErrResourceLimit
		}
		total += n
	}
	return nil
}
