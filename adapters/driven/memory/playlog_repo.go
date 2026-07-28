package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"autodj/domain/playlog"
	"autodj/ports/outbound"
)

// PlayLogRepo implements outbound.PlayLogRepo in memory (append-only). Used by
// the ephemeral repo build and unit tests; the station's real history lives in
// the SQLite repo (Story 7.5). Append is idempotent on Entry.ID.
type PlayLogRepo struct {
	mu      sync.RWMutex
	entries []playlog.Entry
	seen    map[string]struct{}
}

// NewPlayLogRepo returns an empty in-memory play-log repo.
func NewPlayLogRepo() *PlayLogRepo {
	return &PlayLogRepo{seen: map[string]struct{}{}}
}

// Append writes one entry. Idempotent on Entry.ID (re-writing a play is a no-op).
func (r *PlayLogRepo) Append(_ context.Context, e playlog.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.seen[e.ID]; ok {
		return nil
	}
	r.seen[e.ID] = struct{}{}
	r.entries = append(r.entries, e)
	return nil
}

// Since returns every entry with PlayedAt in [from, to), oldest first.
func (r *PlayLogRepo) Since(_ context.Context, from, to time.Time) ([]playlog.Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := r.window(from, to)
	sort.SliceStable(out, func(i, j int) bool { return out[i].PlayedAt.Before(out[j].PlayedAt) })
	return out, nil
}

// Top returns the n most played tracks with PlayedAt in [from, to).
func (r *PlayLogRepo) Top(_ context.Context, from, to time.Time, n int) ([]playlog.TopItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return playlog.TopN(r.window(from, to), n), nil
}

// window returns a copy of the entries in [from, to). Caller holds the lock.
func (r *PlayLogRepo) window(from, to time.Time) []playlog.Entry {
	var out []playlog.Entry
	for _, e := range r.entries {
		if !e.PlayedAt.Before(from) && e.PlayedAt.Before(to) {
			out = append(out, e)
		}
	}
	return out
}

// compile-time: PlayLogRepo satisfies the outbound port.
var _ outbound.PlayLogRepo = (*PlayLogRepo)(nil)
