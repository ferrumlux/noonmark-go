// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/recurrence"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

var (
	ErrSession       = errors.New("invalid isolated development session")
	ErrConflict      = errors.New("session changed; reopen before editing")
	ErrIncomplete    = errors.New("incomplete or uncertain write; session requires inspection")
	ErrCorrupt       = errors.New("corrupt development session")
	ErrLimit         = errors.New("development session size limit")
	ErrLegacySession = fmt.Errorf("%w: development v1 lacks required audit history; create a new v2 session; existing files were preserved", ErrSession)
)

const marker = "noonmark-mygo-development-session-v2\n"
const markerName = "SESSION_IDENTITY"
const maxRevisions = 1024
const maxRecordBytes = 24 << 20
const maxSessionBytes = 128 << 20

type record struct {
	Format       string `json:"format"`
	Revision     int    `json:"revision"`
	ParentSHA256 string `json:"parentSHA256"`
	Payload      []byte `json:"payload"`
}
type diskStore struct {
	ordinaryService *Service
	ordinaryHead    *admittedAggregate
	root            *os.Root
	path            string
	revision        int
	hash            string
	blocked         bool
	fault           func(string) error
	// Optional private I/O substitutes used by isolated tests. Nil keeps the
	// original operation, flags, ordering, and error mapping unchanged.
	openWrite func(string) (storeWriteFile, error)
	readBack  func(string, int) ([]byte, error)
	syncDir   func(*os.Root, string) error
	closeRoot func() error
}

func (d *diskStore) close() error {
	if d.closeRoot != nil {
		return d.closeRoot()
	}
	return d.root.Close()
}
func pathGuard(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrSession
	}
	for p := path; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return ErrSession
			}
		} else if !os.IsNotExist(e) {
			return ErrSession
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return nil
}
func createStore(path string) (*diskStore, error) {
	if e := pathGuard(path); e != nil {
		return nil, e
	}
	if e := os.Mkdir(path, 0700); e != nil {
		return nil, ErrSession
	}
	r, e := os.OpenRoot(path)
	if e != nil {
		return nil, ErrSession
	}
	d := &diskStore{root: r, path: path}
	if e = d.writeExclusive(markerName, []byte(marker)); e != nil {
		r.Close()
		return nil, e
	}
	return d, nil
}
func openStore(path string) (*diskStore, canonical.Value, error) {
	if e := pathGuard(path); e != nil {
		return nil, canonical.Value{}, e
	}
	r, e := os.OpenRoot(path)
	if e != nil {
		return nil, canonical.Value{}, ErrSession
	}
	d := &diskStore{root: r, path: path}
	fail := func(err error) (*diskStore, canonical.Value, error) { r.Close(); return nil, canonical.Value{}, err }
	b, e := d.readRegular(markerName, 128)
	if e == nil && string(b) == "noonmark-mygo-development-session-v1\n" {
		return fail(ErrLegacySession)
	}
	if e != nil || string(b) != marker {
		return fail(ErrSession)
	}
	names, e := d.revisions()
	if e != nil {
		return fail(e)
	}
	if len(names) == 0 {
		return fail(ErrIncomplete)
	}
	var snapshot canonical.Value
	total := 0
	for _, n := range names {
		b, e := d.readRegular(n+"/state.json", maxRecordBytes)
		if e != nil {
			return fail(ErrIncomplete)
		}
		total += len(b)
		if total > maxSessionBytes {
			return fail(ErrLimit)
		}
		digest := sha256.Sum256(b)
		hash := hex.EncodeToString(digest[:])
		ready, e := d.readRegular(n+"/COMMITTED", 65)
		if e != nil || string(ready) != hash+"\n" {
			return fail(ErrIncomplete)
		}
		var rec record
		if _, e = canonical.Parse(b, canonical.Limits{MaxBytes: maxRecordBytes, MaxDepth: 8, MaxNodes: 32}); e != nil {
			return fail(ErrCorrupt)
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if e = dec.Decode(&rec); e != nil {
			return fail(ErrCorrupt)
		}
		if rec.Format != "noonmark-mygo-development-v2" || rec.Revision != d.revision+1 || rec.ParentSHA256 != d.hash {
			return fail(ErrCorrupt)
		}
		snapshot, e = decodeSnapshot(rec.Payload)
		if e != nil {
			return fail(ErrCorrupt)
		}
		d.revision = rec.Revision
		d.hash = hash
	}
	return d, snapshot, nil
}
func openOrdinaryStore(path string) (*diskStore, canonical.Value, error) {
	if e := pathGuard(path); e != nil {
		return nil, canonical.Value{}, e
	}
	r, e := os.OpenRoot(path)
	if e != nil {
		return nil, canonical.Value{}, ErrSession
	}
	d := &diskStore{root: r, path: path}
	fail := func(err error) (*diskStore, canonical.Value, error) { r.Close(); return nil, canonical.Value{}, err }
	b, e := d.readRegular(markerName, 128)
	if e == nil && string(b) == "noonmark-mygo-development-session-v1\n" {
		return fail(ErrLegacySession)
	}
	if e != nil || string(b) != marker {
		return fail(ErrSession)
	}
	names, e := d.revisions()
	if e != nil {
		return fail(e)
	}
	if len(names) == 0 {
		return fail(ErrIncomplete)
	}
	var snapshot canonical.Value
	total := 0
	for _, n := range names {
		b, e := d.readRegular(n+"/state.json", maxRecordBytes)
		if e != nil {
			return fail(ErrIncomplete)
		}
		total += len(b)
		if total > maxSessionBytes {
			return fail(ErrLimit)
		}
		digest := sha256.Sum256(b)
		hash := hex.EncodeToString(digest[:])
		ready, e := d.readRegular(n+"/COMMITTED", 65)
		if e != nil || string(ready) != hash+"\n" {
			return fail(ErrIncomplete)
		}
		var rec record
		if _, e = canonical.Parse(b, canonical.Limits{MaxBytes: maxRecordBytes, MaxDepth: 8, MaxNodes: 32}); e != nil {
			return fail(ErrCorrupt)
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if e = dec.Decode(&rec); e != nil {
			return fail(ErrCorrupt)
		}
		if rec.Format != "noonmark-mygo-development-v2" || rec.Revision != d.revision+1 || rec.ParentSHA256 != d.hash {
			return fail(ErrCorrupt)
		}
		admitted, admissionErr := admitOrdinaryAggregate(rec.Payload, canonical.DefaultLimits())
		e = admissionErr
		if errors.Is(e, ordinary.ErrUnsupported) {
			return fail(e)
		}
		if e == nil {
			snapshot, e = admitted.document.Snapshot()
		}
		if e != nil {
			return fail(ErrCorrupt)
		}
		d.revision = rec.Revision
		d.hash = hash
	}
	return d, snapshot, nil
}
func (d *diskStore) revisions() ([]string, error) {
	f, e := d.root.Open(".")
	if e != nil {
		return nil, ErrSession
	}
	defer f.Close()
	entries, e := f.ReadDir(maxRevisions + 2)
	if e != nil && e != io.EOF {
		return nil, ErrSession
	}
	if len(entries) > maxRevisions+1 {
		return nil, ErrLimit
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() == markerName {
			continue
		}
		n := entry.Name()
		if !entry.IsDir() || len(n) != 14 || !strings.HasPrefix(n, "rev-") {
			return nil, ErrCorrupt
		}
		v, e := strconv.Atoi(n[4:])
		if e != nil || v < 1 || v > maxRevisions {
			return nil, ErrCorrupt
		}
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		if n != fmt.Sprintf("rev-%010d", i+1) {
			return nil, ErrCorrupt
		}
	}
	return names, nil
}
func (d *diskStore) readRegular(name string, limit int) ([]byte, error) {
	info, e := d.root.Lstat(name)
	if e != nil || !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return nil, ErrCorrupt
	}
	f, e := d.root.Open(name)
	if e != nil {
		return nil, ErrCorrupt
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(info, actual) {
		return nil, ErrCorrupt
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if e != nil || len(b) > limit {
		return nil, ErrCorrupt
	}
	return b, nil
}

type storeWriteFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func (d *diskStore) openWriteFile(name string) (storeWriteFile, error) {
	if d.openWrite != nil {
		return d.openWrite(name)
	}
	return d.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}
func (d *diskStore) syncDirectory(name string) error {
	if d.syncDir != nil {
		return d.syncDir(d.root, name)
	}
	return syncDirectory(d.root, name)
}
func (d *diskStore) writeExclusive(name string, b []byte) error {
	f, e := d.openWriteFile(name)
	if e != nil {
		return ErrIncomplete
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	n, e := f.Write(b)
	if e != nil || n != len(b) {
		return ErrIncomplete
	}
	if e = f.Sync(); e != nil {
		return ErrIncomplete
	}
	if e = f.Close(); e != nil {
		return ErrIncomplete
	}
	ok = true
	readBack := d.readRegular
	if d.readBack != nil {
		readBack = d.readBack
	}
	actual, e := readBack(name, len(b))
	if e != nil || !bytes.Equal(actual, b) {
		return ErrIncomplete
	}
	return nil
}
func (d *diskStore) commit(snapshot canonical.Value) error {
	return d.commitObserved(snapshot).err
}
func (d *diskStore) commitObserved(snapshot canonical.Value) commitObservation {
	return d.commitStoreInputObserved(storeWriteInput{legacy: &snapshot})
}

// The shared transaction accepts a closed input, never raw bytes or a caller
// validator. It validates the actual bound handle before acquiring payload bytes.
type storeWriteInput struct {
	legacy   *canonical.Value
	ordinary *persistableOrdinaryCandidate
}

// With an ordinary binding, the caller holds ordinaryService.mu through the
// confirmed disk head and Service publication. This is not a concurrent API.
func (d *diskStore) commitStoreInputObserved(input storeWriteInput) commitObservation {
	extent, stage := WriteNone, StageSessionGate
	invalidates, reason := false, HealthOK
	result := func(err error) commitObservation {
		if invalidates && d.ordinaryService != nil {
			d.ordinaryService.invalidateLocked(reason, stage)
		}
		return commitObservation{extent, stage, invalidates, reason, err}
	}
	if d.blocked {
		invalidates, reason = true, HealthBlocked
		return result(ErrIncomplete)
	}
	if d.revision >= maxRevisions {
		stage = StageCapacity
		return result(ErrLimit)
	}
	stage = StageCandidate
	if (input.legacy == nil) == (input.ordinary == nil) {
		return result(ErrConflict)
	}
	var payload []byte
	var e error
	if input.ordinary != nil {
		payload, e = d.validateOrdinaryWriteInput(*input.ordinary)
	} else {
		if d.ordinaryService != nil || d.ordinaryHead != nil {
			return result(ErrConflict)
		}
		payload, e = encodeSnapshot(*input.legacy)
	}
	if e != nil {
		return result(e)
	}
	stage = StageCapacity
	b, e := json.Marshal(record{Format: "noonmark-mygo-development-v2", Revision: d.revision + 1, ParentSHA256: d.hash, Payload: payload})
	if e != nil || len(b) > maxRecordBytes {
		return result(ErrLimit)
	}
	stage = StagePrefix
	verified := d.verifyPrefixObserved(d.revision, d.hash, false)
	total, e := verified.bytes, verified.err
	if e != nil {
		invalidates, reason = true, HealthPrefixInvalid
		return result(e)
	}
	stage = StageCapacity
	if total > maxSessionBytes-len(b) {
		return result(ErrLimit)
	}
	stage = StageReserve
	if d.fault != nil {
		if e := d.fault("before-reserve"); e != nil {
			return result(e)
		}
	}
	name := fmt.Sprintf("rev-%010d", d.revision+1)
	extent, invalidates, reason = WriteReservationPossible, true, HealthWriteUncertain
	if e = d.root.Mkdir(name, 0700); e != nil {
		return result(ErrConflict)
	}
	extent = WriteReservedOrPartial
	d.blocked = true // any uncertain result after reservation requires a fresh open.
	if d.fault != nil {
		if e := d.fault("after-reserve"); e != nil {
			return result(e)
		}
	}
	stage = StagePayload
	if e = d.writeExclusive(name+"/state.json", b); e != nil {
		return result(e)
	}
	stage = StageMarker
	if d.fault != nil {
		if e := d.fault("before-commit"); e != nil {
			return result(e)
		}
	}
	digest := sha256.Sum256(b)
	hash := hex.EncodeToString(digest[:])
	if e = d.writeExclusive(name+"/COMMITTED", []byte(hash+"\n")); e != nil {
		return result(e)
	}
	// Directory fsync has no portable Go contract on Windows. File fsync and
	// read-back are enforced everywhere; power-loss directory durability remains
	// platform validation work, and is not advertised as a production guarantee.
	stage = StageSync
	if e = d.syncDirectory(name); e != nil {
		return result(e)
	}
	if e = d.syncDirectory("."); e != nil {
		return result(e)
	}
	extent, stage = WriteWrittenUnacknowledged, StagePostPersist
	if d.fault != nil {
		if e := d.fault("after-persist"); e != nil {
			return result(ErrIncomplete)
		}
	}
	stage = StageReadback
	if verified = d.verifyPrefixObserved(d.revision+1, hash, true); verified.err != nil {
		return result(ErrIncomplete)
	}
	d.revision++
	d.hash = hash
	if input.ordinary != nil {
		d.ordinaryHead = &input.ordinary.after
	}
	d.blocked = false
	extent, stage, invalidates, reason = WriteCommittedConfirmed, StageNone, false, HealthOK
	return result(nil)
}
func encodeSnapshot(snapshot canonical.Value) ([]byte, error) {
	if e := ValidateEmptyHistory(snapshot); e != nil {
		return nil, e
	}
	if e := validateRuntimeScope(snapshot); e != nil {
		return nil, e
	}
	return v7.New(v7.Options{ClassificationAuditValidator: ValidateEmptyHistory}).Encode(snapshot)
}
func decodeSnapshot(b []byte) (canonical.Value, error) {
	doc, e := v7.New(v7.Options{ClassificationAuditValidator: ValidateEmptyHistory}).Decode(b)
	if e != nil {
		return canonical.Value{}, e
	}
	snapshot := doc.Snapshot()
	// The generic codec skips audit callbacks for an all-empty classification
	// section. A locked trace still requires empty-history events, so this call
	// is mandatory even when revision is zero.
	if e = ValidateEmptyHistory(snapshot); e != nil {
		return canonical.Value{}, e
	}
	if e = validateRuntimeScope(snapshot); e != nil {
		return canonical.Value{}, e
	}
	return snapshot, nil
}
func validateRuntimeScope(snapshot canonical.Value) error {
	state, extra, e := v7.ToDomain(snapshot)
	if e != nil {
		return e
	}
	neutral, e := emptySupplement()
	if e != nil {
		return e
	}
	for _, pair := range [][2]canonical.Value{{extra.Preferences, neutral.Preferences}, {extra.Ideas, neutral.Ideas}} {
		if !historyEqual(pair[0], pair[1]) {
			return ErrCorrupt
		}
	}
	if len(state.Series) != 0 {
		return core.ErrUnsupportedRecurring
	}
	_, e = core.NewTaskKernel(state.Tasks)
	return e
}
func initialSnapshot() (canonical.Value, error) {
	extra, e := emptySupplement()
	if e != nil {
		return canonical.Value{}, e
	}
	return v7.FromDomain(recurrence.State{}, extra)
}
func normalizeSnapshot(snapshot canonical.Value) (canonical.Value, core.TaskGraph, error) {
	b, e := encodeSnapshot(snapshot)
	if e != nil {
		return canonical.Value{}, core.TaskGraph{}, e
	}
	normalized, e := decodeSnapshot(b)
	if e != nil {
		return canonical.Value{}, core.TaskGraph{}, e
	}
	state, _, e := v7.ToDomain(normalized)
	return normalized, state.Tasks, e
}
func emptySupplement() (v7.Supplement, error) {
	v, e := canonical.Parse([]byte(neutralSupplement), canonical.DefaultLimits())
	if e != nil {
		return v7.Supplement{}, e
	}
	return v7.Supplement{Preferences: v.Object["preferences"], Classifications: v.Object["classifications"], Ideas: canonical.Value{Kind: canonical.Array, Array: []canonical.Value{}}}, nil
}

const neutralSupplement = `{"preferences":{"backupPolicy":{"destination":"iCloudDrive","frequency":"off"},"dataMode":"localFirst","language":"chinese","localFirstSyncPolicy":{"enabled":false,"endpoint":"localFolder","generateConflictCopy":true,"intervalSeconds":300,"mode":"manual","snapshotRetention":{"indexRetentionDays":30,"retentionIndexesDaily":1}},"settingsPoemDisplayPolicy":{"enabled":false,"text":""},"syncEndpointOptions":[],"theme":"coolGray","themeLanguageUpdatedAt":0,"themeLanguageWriterID":"mygo.development"},"classifications":{"categories":[],"categoryDeletionTombstones":[],"categoryMerges":[],"changeRecords":[],"committedReceiptsByInteractionID":[],"currentByChainID":[],"formatVersion":1,"labelDeletionTombstones":[],"labelMerges":[],"labels":[],"relationHistory":[],"revision":0,"snapshotEventsByTraceID":[],"snapshotsByTraceID":[]}}`

// verifyPrefix binds every saved revision back to this handle's expected head.
// A digest-only check of the latest COMMITTED file cannot prove that a session
// remains reopenable: old bytes or the identity marker may have changed.
func (d *diskStore) verifyPrefix(expectedRevision int, expectedHash string, allowSuccessors bool) (int, error) {
	if e := pathGuard(d.path); e != nil {
		return 0, e
	}
	info, e := os.Lstat(d.path)
	if e != nil {
		return 0, ErrSession
	}
	dir, e := d.root.Open(".")
	if e != nil {
		return 0, ErrSession
	}
	actual, se := dir.Stat()
	ce := dir.Close()
	if se != nil || ce != nil || !os.SameFile(info, actual) {
		return 0, ErrSession
	}
	identity, e := d.readRegular(markerName, 128)
	if e != nil || string(identity) != marker {
		return 0, ErrSession
	}
	names, e := d.revisions()
	if e != nil {
		return 0, e
	}
	if len(names) < expectedRevision || !allowSuccessors && len(names) != expectedRevision {
		return 0, ErrConflict
	}
	names = names[:expectedRevision]
	total := 0
	parent := ""
	for i, n := range names {
		b, e := d.readRegular(n+"/state.json", maxRecordBytes)
		if e != nil {
			return 0, ErrCorrupt
		}
		total += len(b)
		if total > maxSessionBytes {
			return 0, ErrLimit
		}
		sum := sha256.Sum256(b)
		digest := hex.EncodeToString(sum[:])
		ready, e := d.readRegular(n+"/COMMITTED", 65)
		if e != nil || string(ready) != digest+"\n" {
			return 0, ErrCorrupt
		}
		if _, e = canonical.Parse(b, canonical.Limits{MaxBytes: maxRecordBytes, MaxDepth: 8, MaxNodes: 32}); e != nil {
			return 0, ErrCorrupt
		}
		var rec record
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if e = dec.Decode(&rec); e != nil {
			return 0, ErrCorrupt
		}
		if rec.Format != "noonmark-mygo-development-v2" || rec.Revision != i+1 || rec.ParentSHA256 != parent {
			return 0, ErrCorrupt
		}
		parent = digest
	}
	if parent != expectedHash {
		return 0, ErrConflict
	}
	return total, nil
}

func (d *diskStore) commitOrdinaryObserved(c persistableOrdinaryCandidate) commitObservation {
	return d.commitStoreInputObserved(storeWriteInput{ordinary: &c})
}

// Caller holds the bound Service mutex. Never lock it recursively here.
func (d *diskStore) validateOrdinaryWriteInput(c persistableOrdinaryCandidate) ([]byte, error) {
	s := d.ordinaryService
	if s == nil || s.disk != d || d.ordinaryHead == nil {
		return nil, ErrConflict
	}
	if err := s.gateLocked(); err != nil {
		return nil, err
	}
	if (c.task == nil) == (c.category == nil) {
		return nil, ErrConflict
	}
	if c.owner == nil || c.owner != s.lineage || c.before == nil || c.before != s.aggregate || c.before != d.ordinaryHead || c.head != d.hash || c.revision != d.revision {
		return nil, ErrConflict
	}
	if err := s.verifyOrdinaryCandidateLocked(c); err != nil {
		return nil, err
	}
	checked, err := v7.EncodeOrdinaryAggregate(c.after.document)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(checked, c.bytes) {
		return nil, ErrCorrupt
	}
	return checked, nil
}
