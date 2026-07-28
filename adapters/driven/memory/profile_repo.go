package memory

import (
	"context"
	"sync"

	"autodj/domain/operatorprofile"
	"autodj/ports/outbound"
)

// ProfileRepo implements outbound.ProfileRepo in memory (user-id keyed). Used by
// the ephemeral repo build and unit tests; the station's real layouts live in the
// SQLite repo (Story 7.4).
type ProfileRepo struct {
	mu       sync.RWMutex
	profiles map[string]operatorprofile.Profile
}

// NewProfileRepo returns an empty in-memory profile repo.
func NewProfileRepo() *ProfileRepo {
	return &ProfileRepo{profiles: map[string]operatorprofile.Profile{}}
}

// Get returns the profile for userID, or outbound.ErrProfileNotFound.
func (r *ProfileRepo) Get(_ context.Context, userID string) (operatorprofile.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[userID]
	if !ok {
		return operatorprofile.Profile{}, outbound.ErrProfileNotFound
	}
	return p, nil
}

// Upsert inserts or replaces the profile keyed by its UserID.
func (r *ProfileRepo) Upsert(_ context.Context, p operatorprofile.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profiles[p.UserID] = p
	return nil
}

// compile-time: ProfileRepo satisfies the outbound port.
var _ outbound.ProfileRepo = (*ProfileRepo)(nil)
