// SPDX-License-Identifier: AGPL-3.0-only
package classification

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	core "github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"math"
	"reflect"
	"sort"
	"sync"
	"unicode/utf8"
)

type Engine struct {
	mu            sync.Mutex
	canonicalizer NameCanonicalizer
	state         Snapshot
}

func New(c NameCanonicalizer) (*Engine, error) {
	if c == nil || c.Version() == "" {
		return nil, ErrUnsupported
	}
	return &Engine{canonicalizer: c, state: Snapshot{CanonicalKeyVersion: c.Version(), Items: map[string]Item{}, Current: map[core.TaskChainID]Current{}, Merges: map[string]Merge{}, Tombstones: map[string]Tombstone{}}}, nil
}
func clone[T any](s T) T {
	b, e := json.Marshal(s)
	if e != nil {
		panic("classification invariant violated")
	}
	var out T
	if json.Unmarshal(b, &out) != nil {
		panic("classification clone failed")
	}
	return out
}
func (e *Engine) Snapshot() Snapshot { e.mu.Lock(); defer e.mu.Unlock(); return clone(e.state) }

// RestoreSnapshot accepts only this port's event-backed format. Replay validates
// derived state, intervals, lifecycle, removal origin and management references.
func RestoreSnapshot(c NameCanonicalizer, s Snapshot, g core.TaskGraph) (*Engine, error) {
	e, err := New(c)
	if err != nil {
		return nil, err
	}
	if s.CanonicalKeyVersion != c.Version() {
		return nil, ErrUnsupported
	}
	for _, r := range s.Records {
		got, err := e.apply(r.Command)
		if err != nil || got != r.Receipt {
			return nil, ErrInvalid
		}
	}
	if !reflect.DeepEqual(e.state, s) {
		return nil, ErrInvalid
	}
	if err := ValidateReferences(s, g); err != nil {
		return nil, err
	}
	return e, nil
}
func ValidateReferences(s Snapshot, g core.TaskGraph) error {
	if core.ValidateTaskGraph(g) != nil {
		return ErrInvalid
	}
	chains := map[core.TaskChainID]bool{}
	traces := map[core.DayTraceID]core.TaskChainID{}
	for _, x := range g.Chains {
		chains[x.ID] = true
	}
	for _, x := range g.Traces {
		traces[x.ID] = x.ChainID
	}
	for id := range s.Current {
		if !chains[id] {
			return ErrInvalid
		}
	}
	for _, h := range s.History {
		if traces[h.TraceID] != h.ChainID {
			return ErrInvalid
		}
	}
	for _, h := range s.RelationHistory {
		if !chains[h.ChainID] {
			return ErrInvalid
		}
	}
	return nil
}
func (e *Engine) Execute(c Command, g core.TaskGraph) (Receipt, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ValidateReferences(e.state, g) != nil {
		return Receipt{}, ErrInvalid
	}
	if c.Operation == SetCurrent || c.Operation == Capture {
		found := false
		for _, x := range g.Chains {
			if x.ID == c.ChainID {
				if !c.At.Finite() || c.At.Before(x.CreatedAt) {
					return Receipt{}, ErrInvalid
				}
				found = true
			}
		}
		if !found {
			return Receipt{}, ErrInvalid
		}
	}
	if c.Operation == Capture {
		found := false
		for _, x := range g.Traces {
			if x.ID == c.TraceID && x.ChainID == c.ChainID && x.Status == c.Status {
				if !c.At.Finite() || c.At.Before(x.CreatedAt) || c.At.Before(x.ContentUpdatedAt) || (x.CompletedAt != nil && c.At.Before(*x.CompletedAt)) || (x.SettledAt != nil && c.At.Before(*x.SettledAt)) {
					return Receipt{}, ErrInvalid
				}
				found = true
			}
		}
		if !found {
			return Receipt{}, ErrInvalid
		}
	}
	return e.apply(c)
}
func (e *Engine) apply(c Command) (Receipt, error) {
	if !utf8.ValidString(c.ColorHex) || !commandShape(c) || c.Source.Validate() != nil || !ValidID(c.InteractionID) || !c.At.Finite() {
		return Receipt{}, ErrInvalid
	}
	if c.Source.Kind != UserDirect && c.Source.Kind != DeterministicDomainAction {
		return Receipt{}, ErrUnsupported
	}
	if c.Source.Kind == UserDirect && !ValidID(c.DecisionID) || c.DecisionID != "" && !ValidID(c.DecisionID) {
		return Receipt{}, ErrInvalid
	}
	for _, r := range e.state.Records {
		if r.Command.InteractionID == c.InteractionID {
			if reflect.DeepEqual(r.Command, c) {
				return r.Receipt, nil
			}
			return Receipt{}, ErrConflict
		}
	}
	if c.ExpectedRevision != e.state.Revision || e.state.Revision == math.MaxUint64 {
		return Receipt{}, ErrConflict
	}
	if len(e.state.Records) > 0 && c.At.Before(e.state.Records[len(e.state.Records)-1].Command.At) {
		return Receipt{}, ErrConflict
	}
	if e.canonicalizer.Version() != e.state.CanonicalKeyVersion {
		return Receipt{}, ErrUnsupported
	}
	next := clone(e.state)
	changed, err := mutate(&next, c, e.canonicalizer)
	if err != nil {
		return Receipt{}, err
	}
	if changed {
		next.Revision++
	}
	r := Receipt{c.InteractionID, next.Revision, changed}
	next.Records = append(next.Records, Record{clone(c), r})
	e.state = next
	return r, nil
}
func eventID(c Command, suffix string) string {
	b := sha256.Sum256([]byte(c.InteractionID + ":" + suffix))
	b[6] = (b[6] & 15) | 128
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%08X-%04X-%04X-%04X-%012X", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func mutate(s *Snapshot, c Command, n NameCanonicalizer) (bool, error) {
	rev := s.Revision + 1
	item, exists := s.Items[c.Item.key()]
	switch c.Operation {
	case Create:
		if !c.Item.valid() || exists {
			return false, ErrConflict
		}
		if _, ok := s.Tombstones[c.Item.key()]; ok {
			return false, ErrConflict
		}
		d, k, err := canonical(n, c.Name)
		if err != nil {
			return false, err
		}
		if reserved(s, c.Item, k) {
			return false, ErrConflict
		}
		s.Items[c.Item.key()] = Item{Ref: c.Item, Name: d, CanonicalKey: k, CanonicalKeyVersion: n.Version(), ColorHex: c.ColorHex, PresentationApproval: UserApproved, Lifecycle: Active, NameVersions: []NameVersion{{ID: eventID(c, "name"), Name: d, CanonicalKey: k, CanonicalKeyVersion: n.Version(), ValidFrom: c.At}}, CreatedAt: c.At, UpdatedAt: c.At}
		return true, nil
	case Rename, Archive, Restore, MergeItems, HardDelete:
		if !c.Item.valid() || !exists || item.Lifecycle == Merged {
			return false, ErrConflict
		}
		switch c.Operation {
		case Rename:
			d, k, err := canonical(n, c.Name)
			if err != nil {
				return false, err
			}
			if d == item.Name {
				return false, nil
			}
			if reserved(s, c.Item, k) || !c.At.After(item.NameVersions[len(item.NameVersions)-1].ValidFrom) {
				return false, ErrConflict
			}
			item.NameVersions[len(item.NameVersions)-1].ValidUntil = &c.At
			item.NameVersions = append(item.NameVersions, NameVersion{ID: eventID(c, "name"), Name: d, CanonicalKey: k, CanonicalKeyVersion: n.Version(), ValidFrom: c.At})
			item.Name = d
			item.CanonicalKey = k
			item.PresentationApproval = UserApproved
		case Archive:
			if item.Lifecycle != Active {
				return false, ErrConflict
			}
			for _, m := range s.Merges {
				if m.Target == c.Item {
					return false, ErrConflict
				}
			}
			item.Lifecycle = Archived
		case Restore:
			if item.Lifecycle != Archived {
				return false, ErrConflict
			}
			item.Lifecycle = Active
		case MergeItems:
			target, ok := s.Items[c.Target.key()]
			if !ok || c.Target.Kind != c.Item.Kind || c.Target == c.Item || target.Lifecycle != Active {
				return false, ErrConflict
			}
			item.Lifecycle = Merged
			s.Merges[c.Item.key()] = Merge{c.Item, c.Target, c.At, rev, c.InteractionID}
			for _, chain := range sortedChains(s.Current) {
				refs := currentRefs(s.Current[chain])
				for i, r := range refs {
					if r == c.Item {
						refs[i] = c.Target
					}
				}
				replace(s, chain, uniqueRefs(refs), c, rev)
			}
		case HardDelete:
			if referenced(s, c.Item) {
				return false, ErrConflict
			}
			delete(s.Items, c.Item.key())
			s.Tombstones[c.Item.key()] = Tombstone{c.Item, c.At, rev, c.InteractionID}
			return true, nil
		}
		item.UpdatedAt = c.At
		s.Items[c.Item.key()] = item
		return true, nil
	case SetCurrent:
		if !ValidID(string(c.ChainID)) {
			return false, ErrInvalid
		}
		refs := []Ref{}
		if c.CategoryID != "" {
			refs = append(refs, CategoryRef(c.CategoryID))
		}
		for _, id := range c.LabelIDs {
			refs = append(refs, LabelRef(id))
		}
		refs = uniqueRefs(refs)
		old := currentRefs(s.Current[c.ChainID])
		for _, r := range refs {
			v, ok := s.Items[r.key()]
			if !r.valid() || !ok || v.Lifecycle == Merged || v.Lifecycle == Archived && !contains(old, r) {
				return false, ErrConflict
			}
		}
		if reflect.DeepEqual(old, refs) {
			return false, nil
		}
		replace(s, c.ChainID, refs, c, rev)
		return true, nil
	case Capture:
		if !ValidID(string(c.ChainID)) || !ValidID(string(c.TraceID)) || !c.Status.Valid() || c.Status == core.TracePending || c.Status == core.TraceCancelledDraft {
			return false, ErrInvalid
		}
		h := TraceSnapshot{ID: eventID(c, "snapshot"), TraceID: c.TraceID, ChainID: c.ChainID, Status: c.Status, CapturedAt: c.At, Revision: rev}
		for _, r := range currentRefs(s.Current[c.ChainID]) {
			i := s.Items[r.key()]
			v := HistoricalValue{r, i.Name, i.ColorHex, i.PresentationApproval}
			if r.Kind == Category {
				h.Category = &v
			} else {
				h.Labels = append(h.Labels, v)
			}
		}
		s.History = append(s.History, h)
		return true, nil
	default:
		return false, ErrUnsupported
	}
}
func reserved(s *Snapshot, owner Ref, key string) bool {
	for _, v := range s.Items {
		if v.Ref.Kind == owner.Kind && v.Ref != owner {
			for _, n := range v.NameVersions {
				if n.CanonicalKey == key {
					return true
				}
			}
		}
	}
	return false
}
func contains(rs []Ref, r Ref) bool {
	for _, x := range rs {
		if x == r {
			return true
		}
	}
	return false
}
func uniqueRefs(rs []Ref) []Ref {
	out := []Ref{}
	for _, r := range rs {
		if !contains(out, r) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}
func currentRefs(c Current) []Ref {
	out := []Ref{}
	if c.Category != nil {
		out = append(out, c.Category.Item)
	}
	for _, r := range c.Labels {
		out = append(out, r.Item)
	}
	return uniqueRefs(out)
}
func sortedChains(m map[core.TaskChainID]Current) []core.TaskChainID {
	ids := []core.TaskChainID{}
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
func replace(s *Snapshot, chain core.TaskChainID, refs []Ref, c Command, rev uint64) {
	old := s.Current[chain]
	all := append([]Relation{}, old.Labels...)
	if old.Category != nil {
		all = append(all, *old.Category)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Item.key() < all[j].Item.key() })
	origins := map[Ref]Relation{}
	for _, r := range all {
		origins[r.Item] = r
		if !contains(refs, r.Item) {
			s.RelationHistory = append(s.RelationHistory, RelationHistory{ID: eventID(c, fmt.Sprintf("remove:%s:%s", chain, r.Item.key())), ChainID: chain, Origin: r, RemovedBySource: c.Source, RemovedByDecisionID: c.DecisionID, RemovedAt: c.At, RemovedRevision: rev})
		}
	}
	next := Current{}
	for _, r := range refs {
		rel, ok := origins[r]
		if !ok {
			rel = Relation{r, c.Source, c.DecisionID, c.At, c.At, rev}
		}
		if r.Kind == Category {
			next.Category = &rel
		} else {
			next.Labels = append(next.Labels, rel)
		}
	}
	s.Current[chain] = next
}
func referenced(s *Snapshot, r Ref) bool {
	for _, v := range s.Current {
		if contains(currentRefs(v), r) {
			return true
		}
	}
	for _, v := range s.RelationHistory {
		if v.Origin.Item == r {
			return true
		}
	}
	for _, v := range s.History {
		if v.Category != nil && v.Category.Item == r {
			return true
		}
		for _, l := range v.Labels {
			if l.Item == r {
				return true
			}
		}
	}
	for _, m := range s.Merges {
		if m.Source == r || m.Target == r {
			return true
		}
	}
	return false
}
func (e *Engine) ResolveName(kind Kind, name string) (Ref, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, key, err := canonical(e.canonicalizer, name)
	if err != nil {
		return Ref{}, err
	}
	for _, v := range e.state.Items {
		if v.Ref.Kind == kind {
			for _, n := range v.NameVersions {
				if n.CanonicalKey == key {
					r := v.Ref
					for {
						m, ok := e.state.Merges[r.key()]
						if !ok {
							return r, nil
						}
						r = m.Target
					}
				}
			}
		}
	}
	return Ref{}, ErrInvalid
}

func commandShape(c Command) bool {
	v := Command{Operation: c.Operation, Source: c.Source, InteractionID: c.InteractionID, DecisionID: c.DecisionID, At: c.At, ExpectedRevision: c.ExpectedRevision}
	switch c.Operation {
	case Create:
		v.Item = c.Item
		v.Name = c.Name
		v.ColorHex = c.ColorHex
	case Rename:
		v.Item = c.Item
		v.Name = c.Name
	case Archive, Restore, HardDelete:
		v.Item = c.Item
	case MergeItems:
		v.Item = c.Item
		v.Target = c.Target
	case SetCurrent:
		v.ChainID = c.ChainID
		v.CategoryID = c.CategoryID
		v.LabelIDs = c.LabelIDs
	case Capture:
		v.ChainID = c.ChainID
		v.TraceID = c.TraceID
		v.Status = c.Status
	default:
		return false
	}
	return reflect.DeepEqual(v, c)
}
