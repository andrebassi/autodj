package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"autodj/domain/staffschedule"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// StaffScheduleService owns the announcer roster (the human grid, Story 9.1). It
// composes the StaffScheduleRepo and a Clock: AddShift stamps a deterministic id
// from the clock plus a monotonic counter (like RequestService) and revalidates
// the candidate against the existing roster so the "only one announcer on air at
// a time" invariant (staffschedule.ErrOverlap) is enforced before persisting.
// This is organizational metadata — no audio, independent of the two-engine
// invariant.
type StaffScheduleService struct {
	repo  outbound.StaffScheduleRepo
	clock outbound.Clock

	mu  sync.Mutex
	seq int64
}

// NewStaffScheduleService wires the service over the repo and a clock.
func NewStaffScheduleService(repo outbound.StaffScheduleRepo, clock outbound.Clock) *StaffScheduleService {
	return &StaffScheduleService{repo: repo, clock: clock}
}

// nextID returns a unique id derived from the clock and a monotonic counter.
func (s *StaffScheduleService) nextID(nowNano int64) string {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()
	return fmt.Sprintf("staff-%d-%d", nowNano, seq)
}

// AddShift validates the candidate shift, rebuilds the roster with it (which
// detects same-weekday overlap → staffschedule.ErrOverlap), then persists it.
func (s *StaffScheduleService) AddShift(ctx context.Context, announcerID, announcerName string, weekday time.Weekday, startMin, endMin int) (staffschedule.Shift, error) {
	now := s.clock.Now()
	shift, err := staffschedule.NewShift(s.nextID(now.UnixNano()), announcerID, announcerName, weekday, startMin, endMin)
	if err != nil {
		return staffschedule.Shift{}, err
	}
	existing, err := s.repo.List(ctx)
	if err != nil {
		return staffschedule.Shift{}, err
	}
	if _, err := staffschedule.BuildRoster(append(existing, shift)); err != nil {
		return staffschedule.Shift{}, err
	}
	if err := s.repo.Save(ctx, shift); err != nil {
		return staffschedule.Shift{}, err
	}
	return shift, nil
}

// RemoveShift deletes a shift by id (outbound.ErrShiftNotFound if unknown).
func (s *StaffScheduleService) RemoveShift(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// Roster returns the full validated roster ordered by (weekday, start).
func (s *StaffScheduleService) Roster(ctx context.Context) (staffschedule.Roster, error) {
	shifts, err := s.repo.List(ctx)
	if err != nil {
		return staffschedule.Roster{}, err
	}
	return staffschedule.BuildRoster(shifts)
}

// OnAir returns the shift covering the given instant, and false when nobody is
// scheduled then.
func (s *StaffScheduleService) OnAir(ctx context.Context, at time.Time) (staffschedule.Shift, bool, error) {
	roster, err := s.Roster(ctx)
	if err != nil {
		return staffschedule.Shift{}, false, err
	}
	shift, ok := roster.OnAir(at.Weekday(), staffschedule.MinuteOfDay(at))
	return shift, ok, nil
}

var _ inbound.StaffScheduleService = (*StaffScheduleService)(nil)
