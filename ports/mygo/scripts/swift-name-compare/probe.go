// SPDX-License-Identifier: AGPL-3.0-only
// Offline public-API probe. No Swift implementation or capture is simulated.
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	c "github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
)

type vector struct {
	ID    string `json:"id"`
	Input string `json:"input"`
}
type pair struct {
	ID    string `json:"id"`
	Left  string `json:"left"`
	Right string `json:"right"`
}
type vectors struct {
	Schema     int      `json:"schemaVersion"`
	Provenance string   `json:"provenance"`
	Cases      []vector `json:"cases"`
	Pairs      []pair   `json:"pairs"`
}
type textRecord struct {
	Text    string  `json:"text"`
	Hex     string  `json:"utf8Hex"`
	Scalars []int32 `json:"unicodeScalars"`
}

func record(s string) textRecord {
	r := []rune(s)
	if r == nil {
		r = []rune{}
	}
	return textRecord{s, hex.EncodeToString([]byte(s)), r}
}
func must(s string, e error) string {
	if e != nil {
		panic(e)
	}
	return s
}
func main() {
	if len(os.Args) != 1 {
		panic("no arguments")
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		panic("input size")
	}
	var v vectors
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&v); err != nil {
		panic(err)
	}
	if v.Schema != 1 || v.Provenance != "fixed-synthetic-only" || len(v.Cases) != 108 || len(v.Pairs) != 17 {
		panic("fixed input required")
	}
	cases := []map[string]any{}
	index := map[string]map[string]any{}
	entities := []map[string]any{}
	selected := map[string]bool{"ascii": true, "empty": true, "only-di": true, "display-non-idempotent": true, "space-mark": true, "compat-diaeresis": true}
	for _, v := range v.Cases {
		display := must(c.DisplayName(v.Input))
		key := must(c.CanonicalKey(v.Input))
		twice := must(c.DisplayName(display))
		entityKey := must(c.CanonicalKey(display))
		item := map[string]any{"id": v.ID, "input": record(v.Input), "displayName": record(display), "canonicalKey": record(key), "displayNameOfDisplayName": record(twice), "canonicalKeyOfDisplayName": record(entityKey)}
		if _, ok := index[v.ID]; ok {
			panic("duplicate input")
		}
		index[v.ID] = item
		cases = append(cases, item)
		if selected[v.ID] {
			for _, kind := range []string{"TaskCategory", "TaskLabel"} {
				entities = append(entities, map[string]any{"caseID": v.ID, "entity": kind, "name": record(display), "canonicalKey": record(entityKey), "canonicalKeyVersion": c.AlgorithmVersion})
			}
		}
	}
	pairs := []map[string]any{}
	for _, p := range v.Pairs {
		a, aok := index[p.Left]
		b, bok := index[p.Right]
		if !aok || !bok {
			panic("pair reference")
		}
		item := map[string]any{"id": p.ID, "left": p.Left, "right": p.Right}
		for _, field := range []string{"input", "displayName", "canonicalKey"} {
			left := a[field].(textRecord).Text
			right := b[field].(textRecord).Text
			eq, e := c.Equivalent(left, right)
			if e != nil {
				panic(e)
			}
			item[field] = map[string]bool{"swiftEqual": eq, "utf8Equal": left == right}
		}
		pairs = append(pairs, item)
	}
	result := map[string]any{"schemaVersion": 1, "kind": "go-canonical16-public-api-probe", "goVersion": runtime.Version(), "unicodeVersion": c.UnicodeVersion, "algorithmVersion": c.AlgorithmVersion, "cases": cases, "pairs": pairs, "entityFields": entities}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
