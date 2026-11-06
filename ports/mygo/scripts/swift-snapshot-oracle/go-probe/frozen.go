// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

const registryHash = "b15a50ddaa5315913aceb3d2b14bb823caf57580c7dd8e9c2db32bb71914032a"
const schemaHash = "7a0940e9a1fbc7b847aaa7ea059042e008faaac74b132b8300d0fbb35341d07c"
const vectorsHash = "8651e7dab7dbb359202c0d84d8ad6e02010f4b5e7e8f280208105d9a33c48a5e"

type field struct {
	Wire, Type, M1 string
	Optional       bool
}
type schema struct {
	Objects map[string][]field
	Aliases map[string]string
}
type seed struct{ ID, File, SHA256 string }
type caseSpec struct {
	ID, Seed string
	Steps    []command
	Required bool
}
type textValue struct {
	Present bool
	UTF8Hex string
}
type command struct {
	Op, CommandKey, ChainGoID, DefinitionGoID, NoteGoID, TitleUTF8Hex, NoteVectorID, NowBits string
	Description                                                                              textValue
	ChainRawID, TraceGoID, DayGoID                                                           string
	Date, Today                                                                              dateValue
	ChildrenGoIDs                                                                            []struct{ LineageRawID, ID string }
}
type dateValue struct{ Year, Month, Day int }
type vector struct {
	ID            string
	Input         textValue
	GoInitialNote struct {
		Present     bool
		BodyUTF8Hex string
	}
}
type frozen struct {
	sourceDir string
	Schema    schema
	Seeds     map[string]seed
	Cases     []caseSpec
	Vectors   map[string]vector
	CaseRaw   map[string]any
}

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func readBounded(path string, n int) ([]byte, error) {
	fi, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0 {
		return nil, fail("unsafe-file")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(n)+1))
	if e == nil && len(b) > n {
		e = fail("resource-limit:bytes")
	}
	return b, e
}
func frozenRead(path, hash string, l limits, dst any) (any, error) {
	b, e := readBounded(path, l.Bytes)
	if e != nil {
		return nil, e
	}
	if digest(b) != hash {
		return nil, fail("source-mismatch")
	}
	v, e := parseIngress("config-"+filepath.Base(path), b, l)
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, dst)
	return v, e
}
func loadFrozen(dir string) (frozen, error) {
	f := frozen{sourceDir: dir, Seeds: map[string]seed{}, Vectors: map[string]vector{}, CaseRaw: map[string]any{}}
	_, e := frozenRead(filepath.Join(dir, "FieldSchema-R2.json"), schemaHash, limits{65536, 8, 4096, 256, 49152, 32, 19}, &f.Schema)
	if e != nil {
		return f, e
	}
	var r struct {
		Seeds []seed
		Cases []caseSpec
	}
	rv, e := frozenRead(filepath.Join(dir, "SeedRegistry-R2.json"), registryHash, limits{65536, 12, 4096, 8192, 32768, 32, 18}, &r)
	if e != nil {
		return f, e
	}
	for _, s := range r.Seeds {
		f.Seeds[s.ID] = s
	}
	f.Cases = r.Cases
	for _, c := range rv.(map[string]any)["cases"].([]any) {
		m := c.(map[string]any)
		f.CaseRaw[m["id"].(string)] = m
	}
	var vs []vector
	_, e = frozenRead(filepath.Join(dir, "NoteVectors-R2.json"), vectorsHash, limits{16384, 6, 256, 8192, 32768, 32, 11}, &vs)
	if e != nil {
		return f, e
	}
	for _, v := range vs {
		f.Vectors[v.ID] = v
	}
	return f, nil
}
func (f frozen) matchCase(b []byte) (caseSpec, error) {
	v, e := parseIngress("config-case", b, limits{16384, 12, 4096, 8192, 32768, 32, 18})
	if e != nil {
		return caseSpec{}, e
	}
	for _, c := range f.Cases {
		if reflect.DeepEqual(v, f.CaseRaw[c.ID]) {
			return c, nil
		}
	}
	return caseSpec{}, fail("registry-mismatch")
}
