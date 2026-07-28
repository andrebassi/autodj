package memory

import (
	"context"
	"sync"

	"autodj/domain/youtube"
)

// YouTubeAccountRepo implements outbound.YouTubeAccountRepo in memory. It starts
// unlinked (Load returns youtube.ErrNoAccount); Save replaces the account, Delete
// unlinks. Used by the ephemeral repo build and unit tests.
type YouTubeAccountRepo struct {
	mu      sync.RWMutex
	acc     youtube.Account
	present bool
}

// NewYouTubeAccountRepo returns an unlinked YouTubeAccountRepo.
func NewYouTubeAccountRepo() *YouTubeAccountRepo {
	return &YouTubeAccountRepo{}
}

// Load returns the linked account, or youtube.ErrNoAccount when none is stored.
func (r *YouTubeAccountRepo) Load(context.Context) (youtube.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.present {
		return youtube.Account{}, youtube.ErrNoAccount
	}
	return r.acc, nil
}

// Save links (or replaces) the account.
func (r *YouTubeAccountRepo) Save(_ context.Context, a youtube.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acc, r.present = a, true
	return nil
}

// Delete unlinks the account. Idempotent.
func (r *YouTubeAccountRepo) Delete(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acc, r.present = youtube.Account{}, false
	return nil
}
