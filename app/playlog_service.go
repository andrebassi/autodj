package app

import (
	"context"
	"time"

	"autodj/domain/library"
	"autodj/domain/playlog"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// PlayLogService records what played and serves the history and "most played"
// ranking (Story 7.5). It composes the append-only PlayLogRepo and a Clock (so
// the rolling ranking windows are deterministic in tests — the domain never
// reads the wall clock). The domain owns the immutable Entry construction and
// the aggregation; this service only orchestrates persistence and resolves the
// named period into a concrete [from, now) window.
type PlayLogService struct {
	repo  outbound.PlayLogRepo
	clock outbound.Clock
}

// NewPlayLogService wires the service over the play-log repo and a clock.
func NewPlayLogService(repo outbound.PlayLogRepo, clock outbound.Clock) *PlayLogService {
	return &PlayLogService{repo: repo, clock: clock}
}

// Record snapshots a track that just played at playedAt into the append-only
// log. The domain derives Broadcasted/ECADRelevant from the source, so a
// Spotify play is stored as neither broadcast nor ECAD-relevant. Returns
// playlog.ErrBadEntry for a track with no id or a zero playedAt.
func (s *PlayLogService) Record(ctx context.Context, t library.Track, playedAt time.Time) error {
	e, err := playlog.NewEntry(t, playedAt)
	if err != nil {
		return err
	}
	return s.repo.Append(ctx, e)
}

// History returns the entries played in [from, to), oldest first.
func (s *PlayLogService) History(ctx context.Context, from, to time.Time) ([]playlog.Entry, error) {
	return s.repo.Since(ctx, from, to)
}

// Top returns the n most played tracks over the named rolling window ending
// now(): "dia" (24h), "semana" (7d) or "mes" (30d). Returns inbound.ErrBadPeriod
// for any other period. n <= 0 is left to the repo (returns an empty ranking).
func (s *PlayLogService) Top(ctx context.Context, period string, n int) ([]playlog.TopItem, error) {
	window, err := periodWindow(period)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	return s.repo.Top(ctx, now.Add(-window), now, n)
}

// periodWindow maps a named period to its rolling duration. Unknown periods
// return inbound.ErrBadPeriod.
func periodWindow(period string) (time.Duration, error) {
	switch period {
	case "dia":
		return 24 * time.Hour, nil
	case "semana":
		return 7 * 24 * time.Hour, nil
	case "mes":
		return 30 * 24 * time.Hour, nil
	default:
		return 0, inbound.ErrBadPeriod
	}
}

// compile-time: PlayLogService satisfies the inbound port.
var _ inbound.PlayLogService = (*PlayLogService)(nil)
