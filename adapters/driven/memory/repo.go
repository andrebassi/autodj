// Package memory is an in-memory TrackRepo used for the walking skeleton, unit
// tests, and the desktop build when no Postgres is configured.
package memory

import (
	"context"
	"errors"
	"sort"
	"sync"

	"autodj/domain/library"
)

// ErrNotFound is returned by Get for an unknown id.
var ErrNotFound = errors.New("memory: track not found")

// TrackRepo implements outbound.TrackRepo backed by a map.
type TrackRepo struct {
	mu     sync.RWMutex
	tracks map[string]*library.Track
}

// New returns an empty TrackRepo.
func New() *TrackRepo {
	return &TrackRepo{tracks: map[string]*library.Track{}}
}

// Upsert inserts or replaces a track by ID.
func (r *TrackRepo) Upsert(_ context.Context, t *library.Track) error {
	if err := t.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracks[t.ID] = clone(t)
	return nil
}

// clone deep-copies a Track so the map never aliases a caller's slice fields
// (Markers); mutating a returned track must not touch the stored one.
func clone(t *library.Track) *library.Track {
	cp := *t
	cp.Markers = append([]library.Marker(nil), t.Markers...)
	return &cp
}

// List returns all tracks ordered by ID (stable output for the UI).
func (r *TrackRepo) List(_ context.Context) ([]*library.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*library.Track, 0, len(r.tracks))
	for _, t := range r.tracks {
		out = append(out, clone(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get returns one track by ID.
func (r *TrackRepo) Get(_ context.Context, id string) (*library.Track, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tracks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(t), nil
}

// Delete removes a track by ID. Returns ErrNotFound if it does not exist.
func (r *TrackRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tracks[id]; !ok {
		return ErrNotFound
	}
	delete(r.tracks, id)
	return nil
}

// DeleteAll wipes the whole catalog (used by the header "Limpar" → start from
// zero). Idempotent: empty repo is a no-op, never errors.
func (r *TrackRepo) DeleteAll(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracks = map[string]*library.Track{}
	return nil
}

// Count returns the number of tracks.
func (r *TrackRepo) Count(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tracks), nil
}
