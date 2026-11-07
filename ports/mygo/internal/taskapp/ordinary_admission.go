// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"fmt"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/v7"
	"github.com/ferrumlux/noonmark/ports/mygo/domain-extensions/classification/ordinary"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// admittedAggregate is only an isolated read-admission result. In particular it
// carries no Service owner, before/head binding or persistence capability.
// The zero value is unusable. Only this constructor defines the runtime profile;
// no caller-supplied validator or claimed proof can turn an import into authority.
type admittedAggregate struct{ document v7.OrdinaryAggregate }

func admitOrdinaryAggregate(data []byte, limits canonical.Limits) (admittedAggregate, error) {
	d, err := v7.DecodeOrdinaryAggregate(data, limits)
	if err != nil {
		return admittedAggregate{}, err
	}
	g, s, err := d.Facts()
	if err != nil {
		return admittedAggregate{}, err
	}
	if err = validateOrdinaryRuntimeProfile(g, s); err != nil {
		return admittedAggregate{}, err
	}
	return admittedAggregate{document: d}, nil
}

// Source-state integrity and this deliberately narrower runtime profile are
// separate. All existing V2 terminal capture cases remain mandatory even when
// the catalog and revision are empty. Historical values are never replaced by
// current catalog values. Stream and revision audit belongs to the source layer.
func validateOrdinaryRuntimeProfile(g core.TaskGraph, s ordinary.AggregateState) error {
	invalid := func(reason string) error { return fmt.Errorf("%w: ordinary runtime %s", ordinary.ErrInvalid, reason) }
	for _, chain := range g.Chains {
		if chain.State != core.ChainActive {
			return ordinary.ErrUnsupported
		}
	}
	for _, category := range s.Categories {
		if category.Lifecycle != "active" || category.PresentationApproval != "userApproved" || len(category.NameVersions) != 1 || category.NameVersions[0].ValidUntil != nil {
			return ordinary.ErrUnsupported
		}
	}
	for _, stream := range s.SnapshotEventsByTraceID {
		for _, event := range stream {
			if event.Category != nil && event.Category.PresentationApproval != "userApproved" {
				return ordinary.ErrUnsupported
			}
		}
	}
	days := make(map[core.LocalDate]core.Day, len(g.Days))
	for _, d := range g.Days {
		days[d.Date] = d
	}
	expected := make(map[core.DayTraceID]core.DayTrace)
	for _, t := range g.Traces {
		if t.Status != core.TracePending && t.Status != core.TraceCompleted && t.Status != core.TraceUnfinished && t.Status != core.TraceReturnedToPool && t.Status != core.TraceCancelledDraft && t.Status != core.TraceDeferred {
			return ordinary.ErrUnsupported
		}
		d, ok := days[t.Date]
		if !ok {
			return invalid("trace day missing")
		}
		if t.Status == core.TraceCancelledDraft {
			if t.DraftCancelledOn == nil || t.Date.Compare(*t.DraftCancelledOn) <= 0 || t.SettledAt == nil || *t.SettledAt != t.ContentUpdatedAt {
				return invalid("cancellation fact")
			}
			continue
		}
		if t.Status == core.TraceReturnedToPool || t.Status == core.TraceDeferred {
			if t.SettledAt == nil || *t.SettledAt != t.ContentUpdatedAt || d.LockedAt != nil && d.LockedAt.Before(*t.SettledAt) {
				return invalid("terminal clock")
			}
			expected[t.ID] = t
			continue
		}
		if d.LockedAt == nil {
			if t.Status == core.TraceUnfinished {
				return invalid("unfinished unlocked trace")
			}
			continue
		}
		if t.Status == core.TracePending {
			return invalid("pending locked trace")
		}
		if d.LockedAt.Before(t.ContentUpdatedAt) || d.LockedAt.Before(t.CreatedAt) {
			return invalid("capture precedes trace")
		}
		if t.Status == core.TraceUnfinished && (t.SettledAt == nil || *t.SettledAt != *d.LockedAt) {
			return invalid("unfinished settlement clock")
		}
		expected[t.ID] = t
	}
	if len(expected) != len(s.SnapshotsByTraceID) || len(expected) != len(s.SnapshotEventsByTraceID) {
		return invalid("missing/extra capture")
	}
	for id, t := range expected {
		stream, ok := s.SnapshotEventsByTraceID[id]
		if !ok || len(stream) == 0 {
			return invalid("missing terminal event")
		}
		if len(stream) != 1 {
			return ordinary.ErrUnsupported
		}
		e := stream[0]
		if e.TraceID != id || e.Status != t.Status {
			return invalid("capture status/trace mismatch")
		}
		at := days[t.Date].LockedAt
		if t.Status == core.TraceReturnedToPool || t.Status == core.TraceDeferred {
			at = t.SettledAt
		}
		if at == nil || e.CapturedAt != *at {
			return invalid("capture clock mismatch")
		}
	}
	return nil
}
