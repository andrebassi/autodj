package memory

import (
	"context"
	"sort"
	"sync"

	"autodj/domain/staffschedule"
	"autodj/ports/outbound"
)

// StaffRepo implements outbound.StaffScheduleRepo in memory (multi-row, id-keyed).
// It starts empty. Used by the ephemeral repo build and unit tests.
type StaffRepo struct {
	mu     sync.RWMutex
	shifts map[string]staffschedule.Shift
}

// NewStaffRepo returns an empty in-memory staff-schedule repo.
func NewStaffRepo() *StaffRepo {
	return &StaffRepo{shifts: map[string]staffschedule.Shift{}}
}

// Save upserts the shift by id.
func (r *StaffRepo) Save(_ context.Context, s staffschedule.Shift) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shifts[s.ID] = s
	return nil
}

// Get returns the shift with id, or outbound.ErrShiftNotFound.
func (r *StaffRepo) Get(_ context.Context, id string) (staffschedule.Shift, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.shifts[id]
	if !ok {
		return staffschedule.Shift{}, outbound.ErrShiftNotFound
	}
	return s, nil
}

// Delete removes the shift by id, or outbound.ErrShiftNotFound if unknown.
func (r *StaffRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.shifts[id]; !ok {
		return outbound.ErrShiftNotFound
	}
	delete(r.shifts, id)
	return nil
}

// List returns all shifts ordered by (weekday, start, id) for stable output.
func (r *StaffRepo) List(context.Context) ([]staffschedule.Shift, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]staffschedule.Shift, 0, len(r.shifts))
	for _, s := range r.shifts {
		out = append(out, s)
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
	return out, nil
}
