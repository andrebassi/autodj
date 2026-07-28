package memory

import (
	"context"
	"sync"

	"autodj/domain/cartwall"
)

// CartwallRepo implements outbound.CartwallRepo in memory. It starts empty (the
// service returns cartwall.Default() for an empty board); Save replaces the
// whole board. Used by the ephemeral repo build and unit tests.
type CartwallRepo struct {
	mu sync.RWMutex
	b  cartwall.Board
}

// NewCartwallRepo returns an empty CartwallRepo.
func NewCartwallRepo() *CartwallRepo {
	return &CartwallRepo{}
}

// Load returns the current board.
func (r *CartwallRepo) Load(context.Context) (cartwall.Board, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.b, nil
}

// Save replaces the stored board.
func (r *CartwallRepo) Save(_ context.Context, b cartwall.Board) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.b = b
	return nil
}
