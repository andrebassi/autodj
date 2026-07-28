// Package stubengine is an in-process PlaybackEngine used in dev when no
// Liquidsoap is running and in tests. It keeps an in-memory now-playing so the
// Player UI and WebSocket still work end-to-end without audio playout.
package stubengine

import (
	"context"
	"sync"

	"autodj/domain/playback"
)

// Engine mirrors pushes into memory and pops on skip.
type Engine struct {
	mu    sync.Mutex
	items []playback.QueueItem
	cur   playback.Playable
}

// New returns an empty stub Engine.
func New() *Engine { return &Engine{} }

// Push appends and, if nothing is playing, promotes it to now-playing. Unlike
// the Liquidsoap engine, the stub accepts any source: it is the personal
// browser-driven playout mirror (no PCM bus of its own), so a non-mixable
// Spotify item rides the same schedule and plays out-of-band via the Web
// Playback SDK. The mix-bus invariant lives in the Liquidsoap adapter.
func (e *Engine) Push(_ context.Context, item playback.QueueItem) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur.TrackID == "" {
		e.cur = item.Playable
		return nil
	}
	e.items = append(e.items, item)
	return nil
}

// Skip pops the next item into now-playing.
func (e *Engine) Skip(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.items) == 0 {
		e.cur = playback.Playable{}
		return nil
	}
	e.cur = e.items[0].Playable
	e.items = e.items[1:]
	return nil
}

// RemoveAt drops the pending item at index i (0 = first queued after cur).
// Out-of-range i is a no-op — keeps the stub aligned with the service mirror
// when the operator removes a queued item.
func (e *Engine) RemoveAt(_ context.Context, i int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i < 0 || i >= len(e.items) {
		return nil
	}
	e.items = append(e.items[:i], e.items[i+1:]...)
	return nil
}

// MoveAt reorders the pending list, moving the item at index from to index to
// (0 = first queued after cur). Out-of-range indices are no-ops; from==to is a
// no-op. Keeps the stub aligned with the service mirror when the operator nudges
// a queued item up/down.
func (e *Engine) MoveAt(_ context.Context, from, to int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(e.items)
	if from < 0 || from >= n || to < 0 || to >= n || from == to {
		return nil
	}
	it := e.items[from]
	e.items = append(e.items[:from], e.items[from+1:]...)
	e.items = append(e.items, playback.QueueItem{})
	copy(e.items[to+1:], e.items[to:])
	e.items[to] = it
	return nil
}

// NowPlaying returns the current in-memory track.
func (e *Engine) NowPlaying(_ context.Context) (playback.Playable, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cur, nil
}

// Healthy is always true for the stub.
func (e *Engine) Healthy(_ context.Context) bool { return true }
