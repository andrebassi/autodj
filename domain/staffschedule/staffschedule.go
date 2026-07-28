// Package staffschedule is the staff-schedule bounded context: the *human* grid
// of the radio — which announcer is on air in which weekday time slot. Unlike
// the program grid (which schedules audio), a Shift carries no audio and never
// touches the playout bus; it is organizational metadata, independent of the
// two-engine invariant. The domain is pure: no clock, no persistence, no
// external deps. Callers pass time in; a Roster aggregates validated shifts and
// enforces the "only one announcer on air at a time" invariant via overlap
// detection.
package staffschedule

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// Domain errors.
var (
	// ErrEmptyID is returned when the shift id is blank.
	ErrEmptyID = errors.New("staffschedule: shift id is required")
	// ErrEmptyAnnouncer is returned when the announcer name is blank.
	ErrEmptyAnnouncer = errors.New("staffschedule: announcer name is required")
	// ErrBadWeekday is returned when weekday is outside 0..6.
	ErrBadWeekday = errors.New("staffschedule: weekday must be 0..6 (Sun..Sat)")
	// ErrBadRange is returned when the minute range is invalid: either endpoint
	// outside 0..1440, or start >= end.
	ErrBadRange = errors.New("staffschedule: minute range must be within 0..1440 with start<end")
	// ErrOverlap is returned when two shifts on the same weekday intersect —
	// only one announcer can be on air at a time.
	ErrOverlap = errors.New("staffschedule: shifts overlap on the same weekday")
)

// minutesPerDay is the exclusive upper bound of a slot (midnight to midnight).
const minutesPerDay = 24 * 60

// Shift is one announcer's on-air slot on a given weekday. StartMin/EndMin are
// minutes since midnight in the half-open interval [StartMin, EndMin). It is a
// value object: methods never mutate the receiver.
type Shift struct {
	ID            string
	AnnouncerID   string
	AnnouncerName string
	Weekday       time.Weekday
	StartMin      int
	EndMin        int
}

// NewShift builds a validated Shift. announcerName is required; announcerID is
// optional (a free-text roster may not have stable ids).
func NewShift(id, announcerID, announcerName string, weekday time.Weekday, startMin, endMin int) (Shift, error) {
	s := Shift{
		ID:            strings.TrimSpace(id),
		AnnouncerID:   strings.TrimSpace(announcerID),
		AnnouncerName: strings.TrimSpace(announcerName),
		Weekday:       weekday,
		StartMin:      startMin,
		EndMin:        endMin,
	}
	if err := s.Validate(); err != nil {
		return Shift{}, err
	}
	return s, nil
}

// Validate reports the first invariant the shift violates.
func (s Shift) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return ErrEmptyID
	}
	if strings.TrimSpace(s.AnnouncerName) == "" {
		return ErrEmptyAnnouncer
	}
	if s.Weekday < time.Sunday || s.Weekday > time.Saturday {
		return ErrBadWeekday
	}
	if s.StartMin < 0 || s.EndMin > minutesPerDay || s.StartMin >= s.EndMin {
		return ErrBadRange
	}
	return nil
}

// Covers reports whether the shift is on air at (weekday, minute), using the
// half-open interval [StartMin, EndMin).
func (s Shift) Covers(weekday time.Weekday, minute int) bool {
	return s.Weekday == weekday && minute >= s.StartMin && minute < s.EndMin
}

// overlaps reports whether two shifts on the same weekday intersect. Adjacency
// (one ending exactly where the next starts) is not an overlap.
func (s Shift) overlaps(o Shift) bool {
	if s.Weekday != o.Weekday {
		return false
	}
	return s.StartMin < o.EndMin && o.StartMin < s.EndMin
}

// Roster is an immutable, validated, overlap-free set of shifts.
type Roster struct {
	shifts []Shift
}

// BuildRoster validates every shift and rejects any same-weekday overlap. The
// stored shifts are sorted by (weekday, start) so Shifts() is deterministic.
func BuildRoster(shifts []Shift) (Roster, error) {
	out := make([]Shift, len(shifts))
	copy(out, shifts)
	for _, s := range out {
		if err := s.Validate(); err != nil {
			return Roster{}, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weekday != out[j].Weekday {
			return out[i].Weekday < out[j].Weekday
		}
		if out[i].StartMin != out[j].StartMin {
			return out[i].StartMin < out[j].StartMin
		}
		return out[i].ID < out[j].ID
	})
	// After sorting by (weekday, start), any overlap is between neighbours.
	for i := 1; i < len(out); i++ {
		if out[i].overlaps(out[i-1]) {
			return Roster{}, ErrOverlap
		}
	}
	return Roster{shifts: out}, nil
}

// Shifts returns the roster's shifts ordered by (weekday, start).
func (r Roster) Shifts() []Shift {
	out := make([]Shift, len(r.shifts))
	copy(out, r.shifts)
	return out
}

// ForWeekday returns the shifts on the given weekday, ordered by start.
func (r Roster) ForWeekday(weekday time.Weekday) []Shift {
	var out []Shift
	for _, s := range r.shifts {
		if s.Weekday == weekday {
			out = append(out, s)
		}
	}
	return out
}

// OnAir returns the shift covering (weekday, minute), if any.
func (r Roster) OnAir(weekday time.Weekday, minute int) (Shift, bool) {
	for _, s := range r.shifts {
		if s.Covers(weekday, minute) {
			return s, true
		}
	}
	return Shift{}, false
}

// MinuteOfDay converts a time to minutes since midnight in its own location.
func MinuteOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
