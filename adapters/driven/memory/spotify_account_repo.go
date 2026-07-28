package memory

import (
	"context"
	"sync"

	"autodj/domain/spotify"
)

// SpotifyAccountRepo implements outbound.SpotifyAccountRepo in memory. It starts
// unlinked (Load returns spotify.ErrNoAccount); Save replaces the account,
// Delete unlinks. Used by the ephemeral repo build and unit tests.
type SpotifyAccountRepo struct {
	mu      sync.RWMutex
	acc     spotify.Account
	present bool
}

// NewSpotifyAccountRepo returns an unlinked SpotifyAccountRepo.
func NewSpotifyAccountRepo() *SpotifyAccountRepo {
	return &SpotifyAccountRepo{}
}

// Load returns the linked account, or spotify.ErrNoAccount when none is stored.
func (r *SpotifyAccountRepo) Load(context.Context) (spotify.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.present {
		return spotify.Account{}, spotify.ErrNoAccount
	}
	return r.acc, nil
}

// Save links (or replaces) the account.
func (r *SpotifyAccountRepo) Save(_ context.Context, a spotify.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acc, r.present = a, true
	return nil
}

// Delete unlinks the account. Idempotent.
func (r *SpotifyAccountRepo) Delete(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acc, r.present = spotify.Account{}, false
	return nil
}
