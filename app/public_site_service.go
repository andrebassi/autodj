package app

import (
	"context"
	"time"

	"autodj/domain/dailyinfo"
	"autodj/domain/publicsite"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// recentWindow is how far back the public site looks for the "já tocou" list.
const recentWindow = 24 * time.Hour

// publicUpcomingLimit / publicRecentLimit cap the public lists so the snapshot
// stays small regardless of queue/history size.
const (
	publicUpcomingLimit = 20
	publicRecentLimit    = 20
)

// PublicSiteService builds the read-only public snapshot (Story 8.4). It composes
// the PlaybackService (now-playing + queue), the append-only PlayLogRepo (recent
// history) and a Clock — it owns no state. The daily info is computed purely from
// the clock, so the whole snapshot is deterministic in tests.
type PublicSiteService struct {
	play  inbound.PlaybackService
	log   outbound.PlayLogRepo
	clock outbound.Clock
}

// NewPublicSiteService wires the service over the playback service, the play-log
// repo and a clock.
func NewPublicSiteService(play inbound.PlaybackService, log outbound.PlayLogRepo, clock outbound.Clock) *PublicSiteService {
	return &PublicSiteService{play: play, log: log, clock: clock}
}

// Snapshot assembles the public view: it reads now-playing + the upcoming queue
// from the playback service, the last 24h of history from the log, computes the
// daily info from the clock, and projects them through publicsite.BuildSnapshot.
// A playback or repo error propagates unchanged.
func (s *PublicSiteService) Snapshot(ctx context.Context) (publicsite.Snapshot, error) {
	now, upcoming, err := s.play.NowPlaying(ctx)
	if err != nil {
		return publicsite.Snapshot{}, err
	}
	at := s.clock.Now()
	recent, err := s.log.Since(ctx, at.Add(-recentWindow), at)
	if err != nil {
		return publicsite.Snapshot{}, err
	}
	daily := dailyinfo.Compute(at)
	return publicsite.BuildSnapshot(now, upcoming, recent, daily, publicUpcomingLimit, publicRecentLimit, at), nil
}

// compile-time: PublicSiteService satisfies the inbound port.
var _ inbound.PublicSiteService = (*PublicSiteService)(nil)
