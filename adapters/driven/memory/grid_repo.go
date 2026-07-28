package memory

import (
	"context"
	"sync"

	"autodj/domain/schedule"
)

// GridRepo implements outbound.GridRepo in memory. It starts empty (no
// programming); Save replaces the whole grid. Used by the ephemeral repo build
// and unit tests.
type GridRepo struct {
	mu sync.RWMutex
	g  schedule.Grid
}

// NewGridRepo returns an empty GridRepo.
func NewGridRepo() *GridRepo {
	return &GridRepo{}
}

// Get returns the current grid.
func (r *GridRepo) Get(context.Context) (schedule.Grid, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.g, nil
}

// Save replaces the stored grid.
func (r *GridRepo) Save(_ context.Context, g schedule.Grid) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.g = g
	return nil
}
