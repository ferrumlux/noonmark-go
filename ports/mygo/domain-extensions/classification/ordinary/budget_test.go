// SPDX-License-Identifier: AGPL-3.0-only
package ordinary

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestBudgetIndependentBoundariesAndCheckedArithmetic(t *testing.T) {
	sample := struct {
		Name   string
		Values []string
	}{"工程", []string{"研究", "学习"}}
	measured, _ := newBudget(DefaultLimits())
	if e := measured.scan(sample); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name   string
		limits Limits
		want   bool
	}{
		{"bytes boundary", Limits{MaxBytes: int(measured.bytes)}, true},
		{"bytes over one", Limits{MaxBytes: int(measured.bytes) - 1}, false},
		{"nodes boundary", Limits{MaxNodes: int(measured.nodes)}, true},
		{"nodes over one", Limits{MaxNodes: int(measured.nodes) - 1}, false},
		{"depth boundary", Limits{MaxDepth: 3}, true},
		{"depth over one", Limits{MaxDepth: 2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, e := newBudget(tc.limits)
			if e != nil {
				t.Fatal(e)
			}
			if got := b.scan(sample) == nil; got != tc.want {
				t.Fatal("boundary", got, tc.want)
			}
		})
	}
	for _, l := range []Limits{{MaxBytes: -1}, {MaxDepth: -1}, {MaxNodes: -1}, {MaxBytes: DefaultLimits().MaxBytes + 1}, {MaxDepth: 129}, {MaxNodes: 1_000_001}} {
		if _, e := newBudget(l); !errors.Is(e, ErrResourceLimit) {
			t.Fatal("invalid config", l, e)
		}
	}
	if _, e := add(math.MaxUint64, 1); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("add overflow")
	}
	if _, e := mul(math.MaxUint64, 2); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("multiply overflow")
	}
	if n, e := mul(math.MaxUint64, 0); e != nil || n != 0 {
		t.Fatal(e)
	}
	b, _ := newBudget(Limits{MaxBytes: 100, MaxNodes: 10})
	if e := b.charge(100, 10); e != nil {
		t.Fatal(e)
	}
	if e := b.charge(1, 0); !errors.Is(e, ErrResourceLimit) {
		t.Fatal(e)
	}
	if b.bytes != 100 || b.nodes != 10 {
		t.Fatal("failed charge changed ledger")
	}
}
func TestBudgetAliasesWholeInputsAndNormalizationWork(t *testing.T) {
	shared := strings.Repeat("x", 8<<20)
	b, _ := newBudget(DefaultLimits())
	if e := b.scan([]string{shared, shared}); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("logical aliases were not counted", e)
	}
	ctx := contextFixture(t, 1)
	state := EmptyState()
	raw := strings.Repeat("\u0315\u0300", 10000)
	if _, e := Prepare(ctx, state, Request{Choice: Choice{Kind: ChoiceNew, Name: raw}}, PrepareIDs{}); e == nil {
		t.Fatal("invalid request unexpectedly valid")
	}
	req := Request{ctx.sealed.graph.Chains[0].ID, Choice{Kind: ChoiceNew, Name: "a" + raw}, uuid(600)}
	if _, e := Prepare(ctx, state, req, PrepareIDs{uuid(601), CategoryID(uuid(602))}); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("normalization work not prepaid", e)
	}
	req.Choice.Name = "\xff" + strings.Repeat("x", 16<<20)
	if _, e := Prepare(ctx, state, req, PrepareIDs{}); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("UTF-8 work before full input scan", e)
	}
	// Both operands and every repeated transform debit the SAME ledger.
	cost := uint64(2 * (64*len("工程") + 2048))
	b, _ = newBudget(Limits{MaxBytes: int(cost)})
	if equal, e := b.equivalent("工程", "工程"); e != nil || !equal {
		t.Fatal(e)
	}
	if _, e := b.equivalent("工程", "工程"); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("Equivalent reset ledger", e)
	}
	b, _ = newBudget(Limits{MaxBytes: 3 * len("工程")})
	if _, e := b.display("工程"); e != nil {
		t.Fatal(e)
	}
	if _, e := b.display("x"); !errors.Is(e, ErrResourceLimit) {
		t.Fatal(e)
	}
}
func TestGraphPreflightIncludesNestedValuesAndCycles(t *testing.T) {
	graph := graphFixture(t, 1)
	graph.Definitions[0].Title = strings.Repeat("x", 16<<20)
	if _, e := NewEmptyContext(graph); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("graph not scanned before validator/clone", e)
	}
	type node struct{ Next *node }
	cycle := &node{}
	cycle.Next = cycle
	b, _ := newBudget(DefaultLimits())
	if e := b.scan(cycle); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("typed scan unbounded cycle", e)
	}
}
func TestWholeReplayTranscriptOutputsAndWorkAreCumulative(t *testing.T) {
	f := newFixture(t, 1)
	f.commit(t, 1, Choice{Kind: ChoiceNew, Name: "工程"}, instant(1))
	if _, e := Replay(f.ctx, f.steps); e != nil {
		t.Fatal("positive", e)
	}
	repeated := make([]CommittedStep, 512)
	for i := range repeated {
		repeated[i] = f.steps[0]
	}
	b, e := entry(f.ctx, repeated)
	if e != nil {
		t.Fatal("input alone should fit", e)
	}
	if e = b.scan(repeated); e != nil {
		t.Fatal("input and transcript copy alone should fit", e)
	}
	if _, e = Replay(f.ctx, repeated); !errors.Is(e, ErrResourceLimit) {
		t.Fatal("repeated hash/validation/output work escaped shared ledger", e)
	}
	small := newFixture(t, 1)
	small.commit(t, 1, Choice{Kind: ChoiceNone}, instant(0))
	proof, e := Replay(small.ctx, small.steps)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Continue(proof, repeated); e == nil {
		t.Fatal("continued transcript should not reset budget or accept another original plan")
	}
}

// This independently checks the exact pinned UCD behind the constants used in
// production. No Swift execution is represented by this static bound proof.
func TestFixedUnicode16ExpansionBounds(t *testing.T) {
	paths := []struct{ name, hash string }{
		{"UnicodeData.txt", "ff58e5823bd095166564a006e47d111130813dcf8bf234ef79fa51a870edb48f"},
		{"DerivedNormalizationProps.txt", "4d4c03892dea9146d674b686e495df2d55a28d071ac474041d73518f887abddc"},
	}
	files := map[string]string{}
	for _, p := range paths {
		data, e := os.ReadFile("../canonical16/testdata/ucd/" + p.name)
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != p.hash {
			t.Fatal("fixed UCD changed; reprove expansion bounds", p.name)
		}
		files[p.name] = string(data)
	}
	decompositions := map[rune][]rune{}
	for _, line := range strings.Split(files["UnicodeData.txt"], "\n") {
		fields := strings.Split(line, ";")
		if len(fields) < 6 || fields[5] == "" || strings.HasPrefix(fields[5], "<") {
			continue
		}
		cp, e := strconv.ParseInt(fields[0], 16, 32)
		if e != nil {
			t.Fatal(e)
		}
		for _, word := range strings.Fields(fields[5]) {
			child, e := strconv.ParseInt(word, 16, 32)
			if e != nil {
				t.Fatal(e)
			}
			decompositions[rune(cp)] = append(decompositions[rune(cp)], rune(child))
		}
	}
	var count func(rune, map[rune]bool) int
	count = func(cp rune, path map[rune]bool) int {
		if path[cp] {
			t.Fatal("cyclic UCD")
		}
		if cp >= 0xac00 && cp <= 0xd7a3 {
			if (cp-0xac00)%28 == 0 {
				return 2
			}
			return 3
		}
		children := decompositions[cp]
		if len(children) == 0 {
			return 1
		}
		path[cp] = true
		n := 0
		for _, child := range children {
			n += count(child, path)
		}
		delete(path, cp)
		return n
	}
	maxCanonical := 3
	for cp := range decompositions {
		n := count(cp, map[rune]bool{})
		if n > maxCanonical {
			maxCanonical = n
		}
	}
	maxFold := 0
	for _, line := range strings.Split(files["DerivedNormalizationProps.txt"], "\n") {
		line = strings.SplitN(line, "#", 2)[0]
		fields := strings.Split(line, ";")
		if len(fields) < 3 || strings.TrimSpace(fields[1]) != "NFKC_CF" {
			continue
		}
		n := len(strings.Fields(fields[2]))
		if n > maxFold {
			maxFold = n
		}
	}
	if maxCanonical != 4 || maxFold != 18 || maxFold*maxCanonical*4 != 288 || maxCanonical*4 != 16 {
		t.Fatalf("unexpected bounds: decomposition=%d fold=%d", maxCanonical, maxFold)
	}
}
