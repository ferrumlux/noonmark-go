// SPDX-License-Identifier: AGPL-3.0-only
package taskapp

import (
	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

// Zero is deliberately invalid for every protocol enumeration.
type Result uint8

const (
	ResultConfirmed Result = iota + 1
	ResultRejected
	ResultUncertain
)

type WriteExtent uint8

const (
	WriteNone WriteExtent = iota + 1
	WriteReservationPossible
	WriteReservedOrPartial
	WriteWrittenUnacknowledged
	WriteCommittedConfirmed
)

type SessionState uint8

const (
	SessionWritable SessionState = iota + 1
	SessionNeedsReopen
	SessionClosed
)

type FailureStage uint8

const (
	StageNone FailureStage = iota + 1
	StageParameter
	StageDomain
	StageCandidate
	StageClock
	StageCapacity
	StagePrefix
	StageReserve
	StagePayload
	StageMarker
	StageSync
	StagePostPersist
	StageReadback
	StageSessionGate
)

type HealthReason uint8

const (
	HealthOK HealthReason = iota + 1
	HealthPrefixInvalid
	HealthWriteUncertain
	HealthBlocked
	HealthProtocolInvalid
)

// SessionHealth contains bounded evidence only, never a stored error or user text.
type SessionHealth struct {
	Session      SessionState
	Reason       HealthReason
	FailureStage FailureStage
}
type Outcome struct {
	Result       Result
	WriteExtent  WriteExtent
	Session      SessionState
	FailureStage FailureStage
	Err          error
}
type AdvanceReceipt struct {
	Revision int
	Hash     string
	Today    core.LocalDate
}
type ManualReceipt struct {
	Revision int
	Hash     string
	Chain    core.TaskChainID
	Trace    core.DayTraceID
	Percent  int
}
type AdvanceOutcome struct {
	Outcome
	Receipt *AdvanceReceipt
}
type ManualOutcome struct {
	Outcome
	Receipt *ManualReceipt
}

// attempt is scoped to one command, under the Service lock. It is never stored
// on Service, and is never reconstructed from a later Health or disk head.
type attempt struct {
	extent WriteExtent
	stage  FailureStage
}

func newAttempt() attempt { return attempt{WriteNone, StageDomain} }
func setAttempt(a []*attempt, extent WriteExtent, stage FailureStage) {
	for _, v := range a {
		if v != nil {
			v.extent, v.stage = extent, stage
		}
	}
}
func (s *Service) Health() SessionHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.health
}
func (s *Service) invalidateLocked(reason HealthReason, stage FailureStage) {
	if s.health.Session == SessionWritable {
		s.health = SessionHealth{SessionNeedsReopen, reason, stage}
	}
}
func (s *Service) gateLocked() error {
	if s.closed {
		return ErrSession
	}
	if s.disk.blocked {
		s.invalidateLocked(HealthBlocked, StageSessionGate)
	}
	if s.health.Session != SessionWritable {
		return ErrIncomplete
	}
	return nil
}

// commitObservation describes the completed disk attempt, not intermediate
// blocked=true during an otherwise successful commit.
type commitObservation struct {
	extent      WriteExtent
	stage       FailureStage
	invalidates bool
	reason      HealthReason
	err         error
}

func (s *Service) commitLocked(wire canonical.Value, a ...*attempt) error {
	o := s.disk.commitObserved(wire)
	setAttempt(a, o.extent, o.stage)
	if o.invalidates {
		s.invalidateLocked(o.reason, o.stage)
	}
	return o.err
}

type verifyObservation struct {
	bytes       int
	err         error
	invalidates bool
}

func (d *diskStore) verifyPrefixObserved(revision int, hash string, successors bool) verifyObservation {
	n, err := d.verifyPrefix(revision, hash, successors)
	return verifyObservation{n, err, err != nil}
}
func (s *Service) verifyPrefixLocked(a ...*attempt) (int, error) {
	o := s.disk.verifyPrefixObserved(s.disk.revision, s.disk.hash, false)
	if o.invalidates {
		s.invalidateLocked(HealthPrefixInvalid, StagePrefix)
	}
	stage := StageNone
	if o.err != nil {
		stage = StagePrefix
	}
	setAttempt(a, WriteNone, stage)
	return o.bytes, o.err
}
func (s *Service) outcomeLocked(a attempt, err error) Outcome {
	o := Outcome{WriteExtent: a.extent, Session: s.health.Session, FailureStage: a.stage, Err: err}
	if err == nil {
		o.Result = ResultConfirmed
		o.FailureStage = StageNone
	} else if a.extent == WriteNone {
		o.Result = ResultRejected
	} else {
		o.Result = ResultUncertain
	}
	if !validOutcome(o, err == nil) {
		s.invalidateLocked(HealthProtocolInvalid, StageSessionGate)
		// Do not invent a receipt or erase possible writes on protocol failure.
		o.Session, o.Err, o.FailureStage = s.health.Session, ErrIncomplete, StageSessionGate
		if a.extent == WriteNone {
			o.Result = ResultRejected
		} else {
			o.Result = ResultUncertain
			o.WriteExtent = WriteReservationPossible
		}
	}
	return o
}

// Validate the complete result including receipt presence, not independent
// enum ranges only. A no-op receipt is still a confirmation.
func validOutcome(o Outcome, receipt bool) bool {
	if o.Session < SessionWritable || o.Session > SessionClosed || o.FailureStage < StageNone || o.FailureStage > StageSessionGate {
		return false
	}
	switch o.Result {
	case ResultConfirmed:
		return o.Err == nil && receipt && o.Session == SessionWritable && o.FailureStage == StageNone && (o.WriteExtent == WriteNone || o.WriteExtent == WriteCommittedConfirmed)
	case ResultRejected:
		return o.Err != nil && !receipt && o.WriteExtent == WriteNone && o.FailureStage != StageNone
	case ResultUncertain:
		return o.Err != nil && !receipt && o.Session == SessionNeedsReopen && o.FailureStage != StageNone && (o.WriteExtent == WriteReservationPossible || o.WriteExtent == WriteReservedOrPartial || o.WriteExtent == WriteWrittenUnacknowledged)
	}
	return false
}
