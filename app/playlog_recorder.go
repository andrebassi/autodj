package app

import (
	"context"
	"sync"

	"autodj/domain/playback"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// PlayLogRecorder turns the live now-playing feed into append-only play-log
// entries (Story 7.5). It observes the current Playable (polled from the engine)
// and records a play the moment the track changes — the AzuraCast on_track
// pattern, adapted to the home box's polling loop. Consecutive observations of
// the same track are deduped so one play logs exactly once. It resolves the full
// Track (for artist/duration/source) from the catalog before recording, so the
// domain still derives Broadcasted/ECADRelevant from the two-engine matrix (a
// Spotify play lands with both false).
type PlayLogRecorder struct {
	log   inbound.PlayLogService
	repo  outbound.TrackRepo
	clock outbound.Clock

	mu   sync.Mutex
	last string // last recorded TrackID (dedup consecutive polls)
}

// NewPlayLogRecorder wires the recorder over the play-log service, the catalog
// (to resolve the full track) and a clock (the play instant).
func NewPlayLogRecorder(log inbound.PlayLogService, repo outbound.TrackRepo, clock outbound.Clock) *PlayLogRecorder {
	return &PlayLogRecorder{log: log, repo: repo, clock: clock}
}

// Observe records p as a new play when its TrackID differs from the last one
// seen. An empty TrackID (nothing playing) and a repeat of the current track are
// ignored. Errors are swallowed: logging must never disrupt playout.
func (r *PlayLogRecorder) Observe(ctx context.Context, p playback.Playable) {
	if p.TrackID == "" {
		return
	}
	r.mu.Lock()
	if p.TrackID == r.last {
		r.mu.Unlock()
		return
	}
	r.last = p.TrackID
	r.mu.Unlock()

	t, err := r.repo.Get(ctx, p.TrackID)
	if err != nil {
		return
	}
	_ = r.log.Record(ctx, *t, r.clock.Now())
}
