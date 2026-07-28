package memory

import (
	"context"
	"sync"

	"autodj/domain/mixing"
)

// MixRepo implements outbound.MixSettingsRepo in memory. It starts at
// mixing.Default(); Save replaces the whole value. Used by the ephemeral repo
// build and unit tests.
type MixRepo struct {
	mu sync.RWMutex
	s  mixing.Settings
}

// NewMixRepo returns a MixRepo seeded with the factory defaults.
func NewMixRepo() *MixRepo {
	return &MixRepo{s: mixing.Default()}
}

// Get returns the current settings.
func (r *MixRepo) Get(context.Context) (mixing.Settings, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.s, nil
}

// Save replaces the stored settings.
func (r *MixRepo) Save(_ context.Context, s mixing.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s = s
	return nil
}
