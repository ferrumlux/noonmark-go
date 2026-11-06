// SPDX-License-Identifier: AGPL-3.0-only
// Synthetic M1 observer only. Never imports taskapp or reads live application data.
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type artifact struct {
	Name     string `json:"name"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
	RawState string `json:"rawState"`
}
type point struct {
	Returned                    map[string]string `json:"returned"`
	Stages                      map[string]string `json:"stages"`
	Key                         string            `json:"key"`
	Artifacts                   []artifact        `json:"artifacts"`
	SelfEqual                   bool              `json:"selfEqual"`
	Kernel, FromDomain, Package []byte            `json:"-"`
}
type result struct {
	SchemaVersion   int     `json:"schemaVersion"`
	FixtureValidity string  `json:"fixtureValidity"`
	CaseID          string  `json:"caseID"`
	Points          []point `json:"points"`
	InteropAccepted bool    `json:"interopAccepted"`
}
type archive struct {
	Root         string
	count, total int
}

func (a *archive) save(point, layer string, b []byte) (string, error) {
	if point != "seed" && point != "create-one" && point != "schedule-one" {
		return "", fail("unsafe-point")
	}
	allowed := map[string]bool{"go-kernel-projection": true, "go-from-domain-projection": true, "go-package": true, "go-export": true, "go-self-reencoded": true}
	if !allowed[layer] {
		return "", fail("unsafe-layer")
	}
	if len(b) > 262144 || a.count >= 18 || a.total+len(b) > 4*1024*1024 {
		return "", fail("resource-limit:archive")
	}
	dir := filepath.Join(a.Root, point)
	if e := os.Mkdir(dir, 0700); e != nil && !os.IsExist(e) {
		return "", e
	}
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", fail("unsafe-directory")
	}
	name := point + "/" + layer + ".json"
	f, e := os.OpenFile(filepath.Join(a.Root, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return "", e
	}
	n, e := f.Write(b)
	ce := f.Close()
	if e != nil || n != len(b) {
		return "", fail("archive-write-failed")
	}
	if ce != nil {
		return "", ce
	}
	a.count++
	a.total += len(b)
	return name, nil
}

var codecLimits = canonical.Limits{MaxBytes: 262144, MaxDepth: 24, MaxNodes: 8192}

func newCodec() *v7.Codec {
	return v7.New(v7.Options{Limits: codecLimits, ClassificationAuditValidator: nil})
}
func emit(f frozen, k *core.TaskKernel, extra v7.Supplement, key string, a *archive) (point, error) {
	p := point{Key: key, Returned: map[string]string{}, Stages: map[string]string{}}
	for _, name := range []string{"preflight", "seed-prepare", "seed-decode", "seed-core", "command", "snapshot", "from-domain", "encode-package", "encode-export", "self-decode", "self-encode", "cross-preflight", "cross-inspect", "cross-decode"} {
		p.Stages[name] = "not-run"
	}
	p.Stages["preflight"] = "success"
	if key == "seed" {
		for _, name := range []string{"seed-prepare", "seed-decode", "seed-core"} {
			p.Stages[name] = "success"
		}
		p.Stages["command"] = "not-applicable"
	} else {
		for _, name := range []string{"seed-prepare", "seed-decode", "seed-core"} {
			p.Stages[name] = "not-applicable"
		}
		p.Stages["command"] = "success"
	}
	store := func(layer string, b []byte) error {
		ar, e := a.observe(key, layer, b, func(raw []byte) (any, error) { return f.Schema.preflightNamed(layer, raw) })
		if ar.Name != "" {
			p.Artifacts = append(p.Artifacts, ar)
		}
		return e
	}
	g := kernelSnapshot(k)
	direct, e := f.Schema.project(g, extra)
	if e != nil {
		return p, e
	}
	if e = store("go-kernel-projection", direct); e != nil {
		return p, e
	}
	p.Kernel = direct
	p.Stages["snapshot"] = "success"
	state := recurrence.State{Tasks: g, Series: []recurrence.Series{}}
	snapshot, e := domainBridge(state, extra)
	if e != nil {
		return p, e
	}
	from, e := canonical.Marshal(canonical.Value{Kind: canonical.Object, Object: map[string]canonical.Value{"formatVersion": {Kind: canonical.Number, TextNumber: "7"}, "snapshot": snapshot}})
	if e != nil {
		return p, e
	}
	p.Stages["from-domain"] = "success"
	if e = store("go-from-domain-projection", from); e != nil {
		return p, e
	}
	p.FromDomain = from
	c := newCodec()
	pkg, e := codecEncode(c, snapshot)
	if e != nil {
		return p, e
	}
	p.Stages["encode-package"] = "success"
	if e = store("go-package", pkg); e != nil {
		return p, e
	}
	p.Package = pkg
	// Export projection is a detached FromDomain snapshot; never changes kernel or seed supplement.
	exp, e := domainBridge(recurrence.State{Tasks: kernelSnapshot(k), Series: []recurrence.Series{}}, extra)
	if e != nil {
		return p, e
	}
	policy := exp.Object["preferences"].Object["localFirstSyncPolicy"]
	policy.Object["enabled"] = canonical.Value{Kind: canonical.Boolean, Bool: false}
	exbytes, e := codecEncode(c, exp)
	if e != nil {
		return p, e
	}
	if e = store("go-export", exbytes); e != nil {
		return p, e
	}
	p.Stages["encode-export"] = "success"
	inspection, e := c.Inspect(pkg)
	if e != nil {
		return p, e
	}
	if inspection.RequiresClassificationAudit {
		return p, fail("unsupported-profile:audit")
	}
	doc, e := codecDecode(c, pkg)
	if e != nil {
		return p, e
	}
	p.Stages["self-decode"] = "success"
	decoded, e := canonical.Marshal(doc.Snapshot())
	if e != nil {
		return p, e
	}
	original, e := canonical.Marshal(snapshot)
	if e != nil {
		return p, e
	}
	if !bytes.Equal(decoded, original) {
		return p, fail("self-relation-difference")
	}
	self, e := codecEncode(c, doc.Snapshot())
	if e != nil {
		return p, e
	}
	if e = store("go-self-reencoded", self); e != nil {
		return p, e
	}
	p.Stages["self-encode"] = "success"
	p.SelfEqual = bytes.Equal(pkg, self)
	return p, nil
}
func textHex(s string) (string, error) {
	if len(s) > 8192 || len(s)%2 != 0 || strings.ToLower(s) != s {
		return "", fail("resource-limit:hex")
	}
	b, e := hex.DecodeString(s)
	if e != nil || !utf8.Valid(b) {
		return "", fail("schema-invalid:utf8-hex")
	}
	return string(b), nil
}
func execute(f frozen, k *core.TaskKernel, c command, before any) error {
	if e := predictCommand(before, c); e != nil {
		return e
	}
	bits, e := strconv.ParseUint(c.NowBits, 16, 64)
	if e != nil {
		return e
	}
	now, e := core.InstantFromBits(bits)
	if e != nil {
		return e
	}
	switch c.Op {
	case "create":
		title, e := textHex(c.TitleUTF8Hex)
		if e != nil {
			return e
		}
		var desc *string
		if c.Description.Present {
			x, e := textHex(c.Description.UTF8Hex)
			if e != nil {
				return e
			}
			desc = &x
		}
		v, ok := f.Vectors[c.NoteVectorID]
		if !ok {
			return fail("registry-mismatch:vector")
		}
		var note *core.TaskNoteEntry
		if v.GoInitialNote.Present {
			body, e := textHex(v.GoInitialNote.BodyUTF8Hex)
			if e != nil {
				return e
			}
			note = &core.TaskNoteEntry{ID: core.TaskNoteEntryID(c.NoteGoID), Body: body, CreatedAt: now, UpdatedAt: now}
		}
		return createCommand(k, core.CreatePoolTaskInput{ChainID: core.TaskChainID(c.ChainGoID), DefinitionID: core.TaskDefinitionID(c.DefinitionGoID), Title: title, DescriptionText: desc, InitialNote: note, Now: now})
	case "schedule":
		ids := map[core.PlannedSubtaskID]core.SubtaskID{}
		g := kernelSnapshot(k)
		found := false
		for _, d := range g.Definitions {
			if string(d.ChainID) != c.ChainRawID {
				continue
			}
			if found {
				return fail("mapping-ambiguous:definition")
			}
			found = true
			for _, p := range d.PlannedSubtasks {
				matches := 0
				for _, id := range c.ChildrenGoIDs {
					if id.LineageRawID == string(p.LineageID) {
						matches++
						ids[p.ID] = core.SubtaskID(id.ID)
					}
				}
				if matches != 1 {
					return fail("mapping-ambiguous:lineage")
				}
			}
		}
		if !found || len(ids) != len(c.ChildrenGoIDs) {
			return fail("mapping-ambiguous:children")
		}
		date := core.LocalDate{Year: c.Date.Year, Month: c.Date.Month, Day: c.Date.Day}
		today := core.LocalDate{Year: c.Today.Year, Month: c.Today.Month, Day: c.Today.Day}
		return scheduleCommand(k, core.ScheduleFromPoolInput{ChainID: core.TaskChainID(c.ChainRawID), TraceID: core.DayTraceID(c.TraceGoID), DayID: core.DayID(c.DayGoID), Date: date, Today: today, Now: now, SubtaskIDs: ids})
	}
	return fail("unsupported-profile:command")
}
func runCase(f frozen, c caseSpec, raw []byte, out string) (result, error) {
	r := result{SchemaVersion: 2, FixtureValidity: "pending", CaseID: c.ID}
	s, ok := f.Seeds[c.Seed]
	if !ok || digest(raw) != s.SHA256 {
		return r, fail("registry-mismatch:seed")
	}
	v, e := f.Schema.preflightNamed("seed-"+c.Seed, raw)
	if e != nil {
		return r, e
	}
	if e = budget(v, 0, 0); e != nil {
		return r, e
	}
	for _, cmd := range c.Steps {
		if e := validateCommandText(f, cmd); e != nil {
			return r, e
		}
	}
	parsed, e := canonical.Parse(raw, codecLimits)
	if e != nil {
		return r, e
	}
	state, extra, e := domainDecode(parsed.Object["snapshot"])
	if e != nil {
		return r, e
	}
	if len(state.Series) != 0 {
		return r, fail("unsupported-profile:series")
	}
	k, e := kernelNew(state.Tasks)
	if e != nil {
		return r, e
	}
	if e = os.Mkdir(out, 0700); e != nil {
		return r, e
	}
	a := archive{Root: out}
	p, e := emit(f, k, extra, "seed", &a)
	r.Points = append(r.Points, p)
	if e != nil {
		return r, e
	}
	r.FixtureValidity = "valid"
	for _, cmd := range c.Steps {
		if e = execute(f, k, cmd, v); e != nil {
			return r, e
		}
		p, e = emit(f, k, extra, cmd.CommandKey, &a)
		if e == nil {
			if cmd.Op == "create" {
				count := 0
				for _, entity := range kernelSnapshot(k).Chains {
					if string(entity.ID) == cmd.ChainGoID {
						count++
					}
				}
				if count != 1 {
					return r, fail("mapping-ambiguous:command-chain")
				}
				p.Returned["chain"] = cmd.ChainGoID
			} else {
				count := 0
				for _, entity := range kernelSnapshot(k).Traces {
					if string(entity.ID) == cmd.TraceGoID {
						count++
					}
				}
				if count != 1 {
					return r, fail("mapping-ambiguous:command-trace")
				}
				p.Returned["trace"] = cmd.TraceGoID
			}
		}
		r.Points = append(r.Points, p)
		if e != nil {
			return r, e
		}
	}
	return r, nil
}

var crossInspectCalls, crossDecodeCalls int
var crossDecoded []byte

func cross(f frozen, b []byte) (string, error) {
	crossDecoded = nil
	if _, e := f.Schema.preflightNamed("cross", b); e != nil {
		return "preflight-rejected", e
	}
	c := newCodec()
	crossInspectCalls++
	i, e := c.Inspect(b)
	if e != nil {
		return "inspect-rejected", e
	}
	if i.RequiresClassificationAudit {
		return "profile-rejected", fail("unsupported-profile:audit")
	}
	crossDecodeCalls++
	d, e := codecDecode(c, b)
	if e != nil {
		return "decode-rejected", e
	}
	crossDecoded, e = canonical.Marshal(canonical.Value{Kind: canonical.Object, Object: map[string]canonical.Value{"formatVersion": {Kind: canonical.Number, TextNumber: "7"}, "snapshot": d.Snapshot()}})
	if e != nil {
		return "projection-rejected", e
	}
	return "accepted", nil
}
func main() {
	spec := flag.String("spec", "", "frozen spec directory")
	fixture := flag.String("fixture", "", "fixed seed file")
	caseFile := flag.String("case", "", "exact registered case JSON")
	vectors := flag.String("vectors", "", "frozen vector file")
	out := flag.String("output", "", "new output directory")
	crossFile := flag.String("cross", "", "raw cross-decode input")
	mutationID := flag.String("mutation-id", "", "fixed negative fixture id")
	matrixMode := flag.String("ingress-matrix", "", "fixed ingress fault matrix")
	flag.Parse()
	if *matrixMode != "" {
		r, e := runIngressMatrix(*spec, *matrixMode, *out)
		if e != nil {
			die(e)
		}
		b, e := json.Marshal(r)
		if e != nil || len(b) > 131072 {
			die(fail("resource-limit:matrix-result"))
		}
		if e = writeExclusive(filepath.Join(*out, "result.json"), b); e != nil {
			die(e)
		}
		fmt.Println(string(b))
		return
	}
	f, e := loadFrozen(*spec)
	if e != nil {
		die(e)
	}

	if *mutationID != "" {
		r, e := runMutation(f, *mutationID)
		if e != nil {
			die(e)
		}
		if *out == "" {
			die(fail("output-required"))
		}
		if e = os.Mkdir(*out, 0700); e != nil {
			die(e)
		}
		b, e := json.Marshal(r)
		if e != nil {
			die(e)
		}
		if e = writeExclusive(filepath.Join(*out, "result.json"), b); e != nil {
			die(e)
		}
		fmt.Println(string(b))
		return
	}
	if *crossFile != "" {
		if *out == "" {
			die(fail("output-required"))
		}
		if e = os.Mkdir(*out, 0700); e != nil {
			die(e)
		}
		b, readErr := readBounded(*crossFile, 262144)
		status := "preflight-rejected"
		var crossErr error = readErr
		if readErr == nil {
			status, crossErr = cross(f, b)
		}
		cr := map[string]any{"schemaVersion": 2, "accepted": crossErr == nil, "requiresClassificationAudit": status == "profile-rejected", "status": status, "inspectCalls": crossInspectCalls, "decodeCalls": crossDecodeCalls}
		if crossErr != nil {
			cr["reason"] = crossErr.Error()
		} else {
			if e = writeExclusive(filepath.Join(*out, "decoded.json"), crossDecoded); e != nil {
				die(e)
			}
			cr["decodedSHA256"] = digest(crossDecoded)
		}
		data, e := json.Marshal(cr)
		if e != nil {
			die(e)
		}
		if e = writeExclusive(filepath.Join(*out, "result.json"), data); e != nil {
			die(e)
		}
		fmt.Println(string(data))
		if crossErr != nil {
			os.Exit(2)
		}
		return
	}

	vb, e := readBounded(*vectors, 16384)
	if e != nil {
		die(e)
	}
	if digest(vb) != vectorsHash {
		die(fail("source-mismatch:vectors"))
	}
	cb, e := readBounded(*caseFile, 16384)
	if e != nil {
		die(e)
	}
	c, e := f.matchCase(cb)
	if e != nil {
		die(e)
	}
	b, e := readBounded(*fixture, 262144)
	if e != nil {
		die(e)
	}
	r, runErr := runCase(f, c, b, *out)
	if runErr != nil {
		if r.FixtureValidity == "pending" {
			r.FixtureValidity = "invalid-fixture"
		}
		encoded, _ := json.Marshal(r)
		if info, se := os.Lstat(*out); se == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			if we := writeExclusive(filepath.Join(*out, "result.json"), encoded); we != nil {
				die(we)
			}
		}
		fmt.Println(string(encoded))
		die(runErr)
	}
	encoded, e := json.Marshal(r)
	if e != nil {
		die(e)
	}
	if len(encoded) > 131072 {
		die(fail("resource-limit:stdout"))
	}
	if e = writeExclusive(filepath.Join(*out, "result.json"), encoded); e != nil {
		die(e)
	}
	fmt.Println(string(encoded))
}
func die(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(2) }

func writeExclusive(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	n, e := f.Write(b)
	ce := f.Close()
	if e != nil {
		return e
	}
	if n != len(b) {
		return fail("short-write")
	}
	return ce
}

// observe deliberately archives bounded complete bytes before calling any output parser/schema.
func (a *archive) observe(key, layer string, b []byte, check func([]byte) (any, error)) (artifact, error) {
	name, e := a.save(key, layer, b)
	if e != nil {
		return artifact{}, e
	}
	ar := artifact{name, len(b), digest(b), "archived-unchecked"}
	_, e = check(b)
	if e != nil {
		ar.RawState = "malformed-raw"
	} else {
		ar.RawState = "schema-valid"
	}
	return ar, e
}
