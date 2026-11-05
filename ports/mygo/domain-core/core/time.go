// SPDX-License-Identifier: AGPL-3.0-only
package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// Instant preserves the exact IEEE-754 bits of Swift Date seconds since
// 2001-01-01T00:00:00Z. Its integer ordering is NOT chronological ordering.
// JSON support is only this scalar, not a complete v7 package codec.
type Instant uint64

func InstantFromSeconds(seconds float64) (Instant, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, ErrInvalidClock
	}
	return Instant(math.Float64bits(seconds)), nil
}
func InstantFromBits(bits uint64) (Instant, error) {
	v := Instant(bits)
	if !v.Finite() {
		return 0, ErrInvalidClock
	}
	return v, nil
}
func (v Instant) Seconds() float64          { return math.Float64frombits(uint64(v)) }
func (v Instant) Finite() bool              { return !math.IsNaN(v.Seconds()) && !math.IsInf(v.Seconds(), 0) }
func (v Instant) Before(other Instant) bool { return v.Seconds() < other.Seconds() }
func (v Instant) After(other Instant) bool  { return v.Seconds() > other.Seconds() }
func (v Instant) Equal(other Instant) bool  { return v.Seconds() == other.Seconds() }
func (v Instant) MarshalJSON() ([]byte, error) {
	if !v.Finite() {
		return nil, ErrInvalidClock
	}
	return []byte(strconv.FormatUint(uint64(v), 10)), nil
}
func (v *Instant) UnmarshalJSON(data []byte) error {
	if v == nil {
		return errors.New("nil Instant receiver")
	}
	// Parsing the decimal integer directly avoids the JSON float64/2^53 trap.
	var bits uint64
	if err := json.Unmarshal(data, &bits); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return ErrInvalidClock
	}
	candidate, err := InstantFromBits(bits)
	if err != nil {
		return err
	}
	*v = candidate
	return nil
}

var ErrInvalidClock = errors.New("invalid or backwards mutation clock")

func clockValid(created, updated Instant, terminal ...*Instant) bool {
	if !created.Finite() || !updated.Finite() || updated.Before(created) {
		return false
	}
	for _, v := range terminal {
		if v != nil && (!v.Finite() || v.Before(created) || v.After(updated)) {
			return false
		}
	}
	return true
}

// NextMutationInstant follows every supplied persisted fact, including after a
// backwards wall-clock adjustment. The caller must supply ALL domain clocks.
func NextMutationInstant(reference Instant, persisted ...Instant) (Instant, error) {
	if !reference.Finite() {
		return 0, ErrInvalidClock
	}
	frontier := reference.Seconds()
	advance := false
	for _, v := range persisted {
		if !v.Finite() {
			return 0, ErrInvalidClock
		}
		if v.Seconds() >= frontier {
			frontier = v.Seconds()
			advance = true
		}
	}
	if advance {
		frontier = math.Nextafter(frontier, math.Inf(1))
	}
	return InstantFromSeconds(frontier)
}

// LocalDate is a Gregorian civil date, never a timestamp or a duration of 24h.
type LocalDate struct {
	Year  int
	Month int
	Day   int
}

func (d LocalDate) Valid() bool {
	if d.Year < 1 || d.Year > 9999 || d.Month < 1 || d.Month > 12 || d.Day < 1 || d.Day > 31 {
		return false
	}
	t := time.Date(d.Year, time.Month(d.Month), d.Day, 12, 0, 0, 0, time.UTC)
	return t.Year() == d.Year && int(t.Month()) == d.Month && t.Day() == d.Day
}
func ParseLocalDate(s string) (LocalDate, error) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return LocalDate{}, errors.New("invalid local date")
	}
	for i, c := range s {
		if i != 4 && i != 7 && (c < '0' || c > '9') {
			return LocalDate{}, errors.New("invalid local date")
		}
	}
	y, _ := strconv.Atoi(s[:4])
	m, _ := strconv.Atoi(s[5:7])
	day, _ := strconv.Atoi(s[8:])
	d := LocalDate{y, m, day}
	if !d.Valid() {
		return LocalDate{}, errors.New("invalid local date")
	}
	return d, nil
}
func (d LocalDate) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }
func (d LocalDate) Compare(other LocalDate) int {
	if d.Year != other.Year {
		return cmpInt(d.Year, other.Year)
	}
	if d.Month != other.Month {
		return cmpInt(d.Month, other.Month)
	}
	return cmpInt(d.Day, other.Day)
}
func (d LocalDate) AddDays(days int) (LocalDate, error) {
	if !d.Valid() || days < -3660000 || days > 3660000 {
		return LocalDate{}, errors.New("invalid date or arithmetic overflow")
	}
	t := time.Date(d.Year, time.Month(d.Month), d.Day, 12, 0, 0, 0, time.UTC).AddDate(0, 0, days)
	result := LocalDate{t.Year(), int(t.Month()), t.Day()}
	if !result.Valid() {
		return LocalDate{}, errors.New("date arithmetic overflow")
	}
	return result, nil
}
func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
