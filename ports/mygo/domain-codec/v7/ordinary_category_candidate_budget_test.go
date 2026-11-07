// SPDX-License-Identifier: AGPL-3.0-only
package v7

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/canonical16"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
)

func TestOrdinaryAdapterMirrorsHardLimits(t *testing.T) {
	a, b := canonical.DefaultLimits(), ordinary.DefaultLimits()
	if a.MaxBytes != b.MaxBytes || a.MaxDepth != b.MaxDepth || a.MaxNodes != b.MaxNodes {
		t.Fatal("ordinary hard limits drifted")
	}
	good := ordinarySnapshot(t)
	for _, limits := range []canonical.Limits{{MaxBytes: -1}, {MaxNodes: -1}, {MaxDepth: -1}, {MaxBytes: a.MaxBytes + 1}, {MaxNodes: a.MaxNodes + 1}, {MaxDepth: a.MaxDepth + 1}} {
		if _, err := ImportOrdinaryCategoryCandidate(good, limits); !errors.Is(err, ordinary.ErrResourceLimit) {
			t.Fatal("bad limits accepted", limits, err)
		}
	}
}
func TestOrdinaryWirePreflightExactIndependentBounds(t *testing.T) {
	// Count escaping, indentation, keys and every shared reference. Each limit is
	// successful at the exact boundary and fails when the input exceeds it by 1.
	shared := objectValue(map[string]canonical.Value{"str": stringValue("工程 / \\ \" \x00\n\b\t\u2028\u2029"), "bits": numberValue("9223372036854775808")})
	root := objectValue(map[string]canonical.Value{"a": ordinaryArray(shared, shared), "b": canonical.Value{Kind: canonical.Boolean, Bool: true}})
	raw, err := canonical.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	cost, err := ordinaryWireScan(root, canonical.DefaultLimits())
	if err != nil || cost.bytes != len(raw) {
		t.Fatal("scan differs from canonical", cost, len(raw), err)
	}
	for _, kind := range []string{"bytes", "nodes", "depth"} {
		limits := canonical.DefaultLimits()
		switch kind {
		case "bytes":
			limits.MaxBytes = cost.bytes
		case "nodes":
			limits.MaxNodes = cost.nodes
		case "depth":
			limits.MaxDepth = 4
		}
		if _, err = ordinaryWireScan(root, limits); err != nil {
			t.Fatal(kind, "boundary rejected", err)
		}
		switch kind {
		case "bytes":
			limits.MaxBytes--
		case "nodes":
			limits.MaxNodes--
		case "depth":
			limits.MaxDepth--
		}
		if _, err = ordinaryWireScan(root, limits); !errors.Is(err, ordinary.ErrResourceLimit) {
			t.Fatal(kind, "plus-one accepted", err)
		}
	}
	single, err := ordinaryWireScan(shared, canonical.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := ordinaryWireScan(ordinaryArray(shared, shared), canonical.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if aliases.nodes != 1+2*single.nodes {
		t.Fatal("shared aliases were deduplicated")
	}
	// Each string fits, their combined byte budget does not.
	if _, err = ordinaryWireScan(ordinaryArray(stringValue(strings.Repeat("x", 40)), stringValue(strings.Repeat("x", 40))), canonical.Limits{MaxBytes: 60, MaxNodes: 100, MaxDepth: 10}); !errors.Is(err, ordinary.ErrResourceLimit) {
		t.Fatal("aggregate bytes not enforced")
	}
}
func TestOrdinaryAdapterBoundedASTAndCustomAggregateLimits(t *testing.T) {
	good := ordinarySnapshot(t)
	root := objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": good})
	if _, err := ordinaryWireScan(root, canonical.DefaultLimits()); err != nil {
		t.Fatal(err)
	}
	// Search the whole import's actual minimal byte budget, including model costs.
	// This is intentionally larger than wire length, and is NOT a promised size.
	lo, hi := 1, canonical.DefaultLimits().MaxBytes
	for lo < hi {
		mid := lo + (hi-lo)/2
		if _, err := ImportOrdinaryCategoryCandidate(good, canonical.Limits{MaxBytes: mid}); err == nil {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	if _, err := ImportOrdinaryCategoryCandidate(good, canonical.Limits{MaxBytes: lo}); err != nil {
		t.Fatal("whole-import boundary", err)
	}
	if _, err := ImportOrdinaryCategoryCandidate(good, canonical.Limits{MaxBytes: lo - 1}); err == nil {
		t.Fatal("whole-import bytes exceeded by one")
	}
	lo, hi = 1, canonical.DefaultLimits().MaxNodes
	for lo < hi {
		mid := lo + (hi-lo)/2
		if _, err := ImportOrdinaryCategoryCandidate(good, canonical.Limits{MaxNodes: mid}); err == nil {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	if _, err := ImportOrdinaryCategoryCandidate(good, canonical.Limits{MaxNodes: lo}); err != nil {
		t.Fatal("whole-import node boundary", err)
	}
	if _, err := ImportOrdinaryCategoryCandidate(good, canonical.Limits{MaxNodes: lo - 1}); err == nil {
		t.Fatal("whole-import nodes exceeded by one")
	}
	cycle := clone(good)
	cycle.Object["cycle"] = cycle
	if _, err := ImportOrdinaryCategoryCandidate(cycle, canonical.Limits{}); err == nil {
		t.Fatal("cyclic object accepted")
	}
	arrayCycle := ordinaryArray(canonical.Value{})
	arrayCycle.Array[0] = arrayCycle
	cycle = clone(good)
	cycle.Object["cycle"] = arrayCycle
	if _, err := ImportOrdinaryCategoryCandidate(cycle, canonical.Limits{}); err == nil {
		t.Fatal("cyclic array accepted")
	}
	inactive := clone(good)
	inactive.String = "hidden"
	if _, err := ImportOrdinaryCategoryCandidate(inactive, canonical.Limits{}); err == nil {
		t.Fatal("inactive union data accepted")
	}
	// Malformed UTF-8 is not repaired by canonical's encoder or the adapter.
	bad := clone(good)
	bad.Object["chains"].Array[0].Object["id"].Object["rawValue"] = stringValue("\xff")
	if _, err := ImportOrdinaryCategoryCandidate(bad, canonical.Limits{}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	// Root versions and unknown envelope fields never reach a model projection.
	for _, mutate := range []func(canonical.Value){func(v canonical.Value) { v.Object["formatVersion"] = numberValue("8") }, func(v canonical.Value) { v.Object["extra"] = ordinaryArray() }} {
		v := clone(root)
		mutate(v)
		raw, err := canonical.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = DecodeOrdinaryCategoryCandidate(raw, canonical.Limits{}); err == nil {
			t.Fatal("bad envelope accepted")
		}
	}
}

func TestOrdinaryAdapterDecodeIntegrityAndReplayAreDifferent(t *testing.T) {
	ctx, state, _ := ordinaryFixture(t)
	// Mutate a valid control by adding an otherwise well-formed orphan entity.
	// State integrity cannot establish that its creation came from a retained plan.
	orphan := state.Categories[ordinary.CategoryID(testID(300))]
	orphan.ID = ordinary.CategoryID(testID(700))
	orphan.NameVersions = append([]ordinary.NameVersion{}, orphan.NameVersions...)
	orphan.Name = "a \u0301b"
	orphan.CanonicalKey, _ = canonical16.CanonicalKey(orphan.Name)
	orphan.NameVersions[0].ID = ordinary.UUID(testID(701))
	orphan.NameVersions[0].Name = orphan.Name
	orphan.NameVersions[0].CanonicalKey = orphan.CanonicalKey
	state.Categories[orphan.ID] = orphan
	if err := ordinary.ValidateApplyIntegrity(ctx, state); err != nil {
		t.Fatal("integrity control", err)
	}
	if err := ordinary.ValidateDecodedEntities(ctx, state); err == nil {
		t.Fatal("entity decoder accepted non-fixed-point display")
	}
	before := orphan.Name
	if _, err := ExportOrdinaryCategoryCandidate(ctx, state); err == nil {
		t.Fatal("round-trip-safe export accepted decoder-invalid entity")
	}
	if state.Categories[orphan.ID].Name != before {
		t.Fatal("adapter normalized away the decode failure")
	}
	// Export is intentionally a round-trip-safe subset. Its rejection above is
	// neither a Swift encode failure nor a core Apply failure or causal proof.
}

func FuzzOrdinaryAdapterBoundedRoundTrip(f *testing.F) {
	good := ordinarySnapshot(f)
	f.Add(ordinaryEnvelope(f, good))
	f.Add([]byte(`{"formatVersion":8}`))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256<<10 {
			return
		}
		original := append([]byte{}, data...)
		candidate, err := DecodeOrdinaryCategoryCandidate(data, canonical.Limits{MaxBytes: 2 << 20, MaxNodes: 100_000})
		if !bytes.Equal(original, data) {
			t.Fatal("input bytes mutated")
		}
		if err != nil {
			return
		}
		wire, err := ExportOrdinaryCategoryCandidate(candidate.Context, candidate.State)
		if err != nil {
			if errors.Is(err, ordinary.ErrResourceLimit) {
				return
			}
			t.Fatal(err)
		}
		again, err := ImportOrdinaryCategoryCandidate(wire, canonical.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(candidate.State, again.State) {
			t.Fatal("accepted state changed on roundtrip")
		}
	})
}
