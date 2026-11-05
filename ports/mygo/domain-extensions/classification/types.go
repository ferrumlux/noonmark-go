// SPDX-License-Identifier: AGPL-3.0-only
// Derived from Noonmark 43e6eefaae3a04be89cab3e234d470111afd80ef.
package classification

import (
	"errors"
	core "github.com/ferrumlux/noonmark/ports/mygo/domain/core"
	"strings"
	"unicode/utf8"
)

type CategoryID string
type LabelID string
type SourceKind string

const (
	UserDirect                SourceKind = "userDirect"
	ZhulongSuggestion         SourceKind = "zhulongSuggestion"
	AutomaticAI               SourceKind = "automaticAI"
	Inherited                 SourceKind = "inherited"
	DeterministicDomainAction SourceKind = "deterministicDomainAction"
)

type Source struct {
	Kind                                  SourceKind
	SessionID, DraftID, EvidenceID, JobID string
	DraftVersion, Generation              int
	FromChainID                           core.TaskChainID
	Reason                                string
}

var ErrInvalid = errors.New("invalid classification fact")
var ErrUnsupported = errors.New("unsupported classification capability or canonicalization")
var ErrConflict = errors.New("classification conflict")

func ValidID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
func (s Source) Validate() error {
	var expected Source
	expected.Kind = s.Kind
	switch s.Kind {
	case UserDirect:
	case ZhulongSuggestion:
		if !ValidID(s.SessionID) || !ValidID(s.DraftID) || !ValidID(s.EvidenceID) || s.DraftVersion <= 0 {
			return ErrInvalid
		}
		expected.SessionID = s.SessionID
		expected.DraftID = s.DraftID
		expected.EvidenceID = s.EvidenceID
		expected.DraftVersion = s.DraftVersion
	case AutomaticAI:
		if !ValidID(s.JobID) || s.Generation <= 0 {
			return ErrInvalid
		}
		expected.JobID = s.JobID
		expected.Generation = s.Generation
	case Inherited:
		if !ValidID(string(s.FromChainID)) {
			return ErrInvalid
		}
		expected.FromChainID = s.FromChainID
	case DeterministicDomainAction:
		if strings.TrimSpace(s.Reason) == "" || !utf8.ValidString(s.Reason) {
			return ErrInvalid
		}
		expected.Reason = s.Reason
	default:
		return ErrInvalid
	}
	if expected != s {
		return ErrInvalid
	}
	return nil
}

// NameCanonicalizer is a trusted, deterministic, version-pinned adapter. Implementations
// must provide Swift-compatible whitespace display normalization and ICU NFKC_Casefold.
// No default lowercasing fallback is used.
type NameCanonicalizer interface {
	Version() string
	Canonicalize(string) (display, key string, err error)
}
type CanonicalName struct{ Display, Key string }

// FixtureCanonicalizer deliberately supports only exact supplied inputs. It is for
// differential fixtures, NOT a production Unicode implementation.
type FixtureCanonicalizer struct {
	Algorithm string
	Names     map[string]CanonicalName
}

func (f FixtureCanonicalizer) Version() string { return f.Algorithm }
func (f FixtureCanonicalizer) Canonicalize(s string) (string, string, error) {
	v, ok := f.Names[s]
	if !ok {
		return "", "", ErrUnsupported
	}
	return v.Display, v.Key, nil
}
func canonical(c NameCanonicalizer, s string) (string, string, error) {
	if c == nil || c.Version() == "" || !utf8.ValidString(s) {
		return "", "", ErrUnsupported
	}
	d, k, e := c.Canonicalize(s)
	if e != nil {
		return "", "", e
	}
	if d == "" || k == "" || !utf8.ValidString(d) || !utf8.ValidString(k) {
		return "", "", ErrInvalid
	}
	return d, k, nil
}

type Kind string

const (
	Category Kind = "category"
	Label    Kind = "label"
)

type Ref struct {
	Kind Kind
	ID   string
}

func CategoryRef(id CategoryID) Ref { return Ref{Category, string(id)} }
func LabelRef(id LabelID) Ref       { return Ref{Label, string(id)} }
func (r Ref) valid() bool           { return (r.Kind == Category || r.Kind == Label) && ValidID(r.ID) }
func (r Ref) key() string           { return string(r.Kind) + ":" + r.ID }

type Lifecycle string

const (
	Active   Lifecycle = "active"
	Archived Lifecycle = "archived"
	Merged   Lifecycle = "merged"
)

type PresentationApproval string

const (
	UserApproved    PresentationApproval = "userApproved"
	PendingAIReview PresentationApproval = "pendingAIReview"
)

type NameVersion struct {
	ID                                      string
	Name, CanonicalKey, CanonicalKeyVersion string
	ValidFrom                               core.Instant
	ValidUntil                              *core.Instant
}
type Item struct {
	Ref                                               Ref
	Name, CanonicalKey, CanonicalKeyVersion, ColorHex string
	PresentationApproval                              PresentationApproval
	Lifecycle                                         Lifecycle
	NameVersions                                      []NameVersion
	CreatedAt, UpdatedAt                              core.Instant
}
type Relation struct {
	Item                 Ref
	Source               Source
	DecisionID           string
	CreatedAt, UpdatedAt core.Instant
	Revision             uint64
}
type Current struct {
	Category *Relation
	Labels   []Relation
}
type RelationHistory struct {
	ID                  string
	ChainID             core.TaskChainID
	Origin              Relation
	RemovedBySource     Source
	RemovedByDecisionID string
	RemovedAt           core.Instant
	RemovedRevision     uint64
}
type HistoricalValue struct {
	Item                 Ref
	Name, ColorHex       string
	PresentationApproval PresentationApproval
}
type TraceSnapshot struct {
	ID         string
	TraceID    core.DayTraceID
	ChainID    core.TaskChainID
	Status     core.TraceStatus
	Category   *HistoricalValue
	Labels     []HistoricalValue
	CapturedAt core.Instant
	Revision   uint64
}
type Merge struct {
	Source, Target Ref
	At             core.Instant
	Revision       uint64
	InteractionID  string
}
type Tombstone struct {
	Item          Ref
	At            core.Instant
	Revision      uint64
	InteractionID string
}
type Operation string

const (
	Create     Operation = "create"
	Rename     Operation = "rename"
	Archive    Operation = "archive"
	Restore    Operation = "restore"
	MergeItems Operation = "merge"
	HardDelete Operation = "hardDelete"
	SetCurrent Operation = "setCurrent"
	Capture    Operation = "capture"
)

// Command is an explicit low-level domain mutation. It does not implement the Swift
// prepare/confirm capability seam. AutomaticAI and ZhulongSuggestion are rejected.
type Command struct {
	Operation                 Operation
	Item, Target              Ref
	Name, ColorHex            string
	ChainID                   core.TaskChainID
	CategoryID                CategoryID
	LabelIDs                  []LabelID
	TraceID                   core.DayTraceID
	Status                    core.TraceStatus
	Source                    Source
	InteractionID, DecisionID string
	At                        core.Instant
	ExpectedRevision          uint64
}
type Receipt struct {
	InteractionID string
	Revision      uint64
	Changed       bool
}
type Record struct {
	Command Command
	Receipt Receipt
}
type Snapshot struct {
	CanonicalKeyVersion string
	Revision            uint64
	Items               map[string]Item
	Current             map[core.TaskChainID]Current
	RelationHistory     []RelationHistory
	History             []TraceSnapshot
	Merges              map[string]Merge
	Tombstones          map[string]Tombstone
	Records             []Record
}
