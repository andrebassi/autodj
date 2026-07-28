package memory

import (
	"context"
	"sort"
	"sync"

	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

// ClockRepo implements outbound.ClockRepo in memory (multi-row, id-keyed). It
// starts empty; the wiring seeds schedule.DefaultClocks() when appropriate. Used
// by the ephemeral repo build and unit tests.
type ClockRepo struct {
	mu     sync.RWMutex
	clocks map[string]schedule.Clock
}

// NewClockRepo returns an empty in-memory clock repo.
func NewClockRepo() *ClockRepo {
	return &ClockRepo{clocks: map[string]schedule.Clock{}}
}

// List returns all clocks ordered by id (stable output).
func (r *ClockRepo) List(context.Context) ([]schedule.Clock, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]schedule.Clock, 0, len(r.clocks))
	for _, c := range r.clocks {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get returns the clock with id, or outbound.ErrClockNotFound.
func (r *ClockRepo) Get(_ context.Context, id string) (schedule.Clock, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clocks[id]
	if !ok {
		return schedule.Clock{}, outbound.ErrClockNotFound
	}
	return c, nil
}

// Save upserts the clock by id.
func (r *ClockRepo) Save(_ context.Context, c schedule.Clock) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clocks[c.ID] = c
	return nil
}

// Delete removes the clock by id, or returns outbound.ErrClockNotFound.
func (r *ClockRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clocks[id]; !ok {
		return outbound.ErrClockNotFound
	}
	delete(r.clocks, id)
	return nil
}
