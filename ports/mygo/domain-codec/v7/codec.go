// SPDX-License-Identifier: AGPL-3.0-only
// Package v7 is an isolated candidate codec, not a production import command.
package v7

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification"
)

const CurrentFormatVersion = 7

// AuditValidator must implement the COMPLETE upstream classification invariants,
// ICU version checks, change-record digests and receipt/revision consistency.
// The second-batch classification.Engine is a different format and is unsuitable.
// The callback must be deterministic and side-effect free.
type AuditValidator func(snapshot canonical.Value) error

type Options struct {
	Limits                       canonical.Limits
	ClassificationAuditValidator AuditValidator
}
type Codec struct{ options Options }

func New(options Options) *Codec {
	if options.Limits == (canonical.Limits{}) {
		options.Limits = canonical.DefaultLimits()
	}
	return &Codec{options: options}
}

// Inspection reports transport/schema/core checks only. It deliberately does
// not certify audit integrity, Swift interoperability, or authorize live import.
type Inspection struct {
	Snapshot                    canonical.Value
	RequiresClassificationAudit bool
}

// Document's facts are private; callers only receive deep copies.
type Document struct{ snapshot canonical.Value }

func (d Document) Snapshot() canonical.Value { return clone(d.snapshot) }

func (c *Codec) Inspect(data []byte) (Inspection, error) {
	value, e := canonical.Parse(data, c.options.Limits)
	if e != nil {
		return Inspection{}, e
	}
	if value.Kind != canonical.Object {
		return Inspection{}, malformed("$", "expected envelope object")
	}
	version, ok := value.Object["formatVersion"]
	if !ok || validateType(version, "int", "$.formatVersion") != nil {
		return Inspection{}, malformed("$.formatVersion", "required integer")
	}
	if signed(version) != CurrentFormatVersion {
		return Inspection{}, fmt.Errorf("%w: %s", ErrUnsupportedVersion, version.TextNumber)
	}
	if e = validateType(value, "Envelope", "$"); e != nil {
		return Inspection{}, e
	}
	expected, e := canonical.Marshal(value)
	if e != nil {
		return Inspection{}, e
	}
	if !bytes.Equal(expected, data) {
		return Inspection{}, malformed("$", "noncanonical candidate bytes")
	}
	snapshot := value.Object["snapshot"]
	if e = validateSnapshot(snapshot); e != nil {
		return Inspection{}, e
	}
	return Inspection{Snapshot: clone(snapshot), RequiresClassificationAudit: hasClassificationAudit(snapshot.Object["classifications"])}, nil
}
func (c *Codec) Decode(data []byte) (Document, error) {
	inspected, e := c.Inspect(data)
	if e != nil {
		return Document{}, e
	}
	if inspected.RequiresClassificationAudit {
		if c.options.ClassificationAuditValidator == nil {
			return Document{}, ErrDomainValidationRequired
		}
		if e = c.options.ClassificationAuditValidator(clone(inspected.Snapshot)); e != nil {
			return Document{}, fmt.Errorf("classification audit: %w", e)
		}
	}
	return Document{snapshot: clone(inspected.Snapshot)}, nil
}
func (c *Codec) Encode(snapshot canonical.Value) ([]byte, error) {
	// Marshal first bounds caller-created graphs and detects cycles before schema
	// walks or clone operations. Never trust programmatically constructed ASTs.
	root := objectValue(map[string]canonical.Value{"formatVersion": numberValue("7"), "snapshot": snapshot})
	data, e := canonical.Marshal(root)
	if e != nil {
		return nil, e
	}
	if _, e = c.Decode(data); e != nil {
		return nil, e
	}
	return data, nil
}
func (c *Codec) Read(reader io.Reader) (Document, error) {
	if reader == nil {
		return Document{}, ErrMalformed
	}
	max := c.options.Limits.MaxBytes
	if max == 0 {
		max = canonical.DefaultLimits().MaxBytes
	}
	if max < 0 || int64(max) == math.MaxInt64 {
		return Document{}, malformed("limits.MaxBytes", "invalid reader bound")
	}
	// LimitReader avoids allocating according to an untrusted length header.
	data, e := io.ReadAll(io.LimitReader(reader, int64(max)+1))
	if e != nil {
		return Document{}, e
	}
	return c.Decode(data)
}

type WriteReceipt struct {
	ByteCount int
	SHA256    string
}

// WriteVerified writes ONLY to caller-owned staging streams and reads them back.
// It does not rename files, choose directories, replace databases or publish data.
// The caller owns atomic installation after success and any OS-specific fsync.
func (c *Codec) WriteVerified(snapshot canonical.Value, writer io.Writer, readBack func() ([]byte, error)) (WriteReceipt, error) {
	if writer == nil || readBack == nil {
		return WriteReceipt{}, ErrWriteVerification
	}
	data, e := c.Encode(snapshot)
	if e != nil {
		return WriteReceipt{}, e
	}
	n, e := writer.Write(data)
	if e != nil {
		return WriteReceipt{}, e
	}
	if n != len(data) {
		return WriteReceipt{}, io.ErrShortWrite
	}
	persisted, e := readBack()
	if e != nil {
		return WriteReceipt{}, e
	}
	if !bytes.Equal(data, persisted) {
		return WriteReceipt{}, ErrWriteVerification
	}
	if _, e = c.Decode(persisted); e != nil {
		return WriteReceipt{}, e
	}
	sum := sha256.Sum256(persisted)
	return WriteReceipt{len(persisted), fmt.Sprintf("%x", sum)}, nil
}
func hasClassificationAudit(c canonical.Value) bool {
	if unsigned(c.Object["revision"]) != 0 {
		return true
	}
	for k, v := range c.Object {
		if k != "formatVersion" && k != "revision" && len(v.Array) > 0 {
			return true
		}
	}
	return false
}
func validateSnapshot(snapshot canonical.Value) error {
	state, _, e := ToDomain(snapshot)
	if e != nil {
		return e
	}
	if strings.TrimSpace(snapshot.Object["preferences"].Object["themeLanguageWriterID"].String) == "" {
		return malformed("snapshot.preferences.themeLanguageWriterID", "empty writer")
	}
	if signed(snapshot.Object["classifications"].Object["formatVersion"]) != 1 {
		return malformed("snapshot.classifications.formatVersion", "unsupported classification state version")
	}
	if e = validateClassificationReferences(snapshot); e != nil {
		return e
	}
	catalogs := catalogIDs(snapshot.Object["classifications"])
	if e = state.ValidateCatalogReferences(func(id classification.CategoryID) bool { return catalogs.categories[string(id)] }, func(id classification.LabelID) bool { return catalogs.labels[string(id)] }); e != nil {
		return e
	}
	return validateIdeas(snapshot.Object["ideas"], catalogs)
}
