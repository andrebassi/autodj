package app

import (
	"context"
	"time"

	"autodj/domain/reports"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// ReportService derives read-only reports from the append-only play log (Story
// 8.1). It composes the PlayLogRepo (the immutable source of truth) and a Clock
// (so GeneratedAt is deterministic in tests — the domain never reads the wall
// clock). The service owns no state: it fetches the window's entries and hands
// them to the pure domain builder, so regenerating a report always agrees.
type ReportService struct {
	repo  outbound.PlayLogRepo
	clock outbound.Clock
}

// NewReportService wires the service over the play-log repo and a clock.
func NewReportService(repo outbound.PlayLogRepo, clock outbound.Clock) *ReportService {
	return &ReportService{repo: repo, clock: clock}
}

// ECAD builds the ECAD execution report over [from, to). It reads the window's
// entries from the append-only log and projects them through reports.BuildECAD,
// which keeps only ECAD-relevant plays (DRM Spotify is excluded) and aggregates
// by track. GeneratedAt comes from the injected clock. Returns
// inbound.ErrBadRange when from is not strictly before to.
func (s *ReportService) ECAD(ctx context.Context, from, to time.Time) (reports.ECADReport, error) {
	if !from.Before(to) {
		return reports.ECADReport{}, inbound.ErrBadRange
	}
	entries, err := s.repo.Since(ctx, from, to)
	if err != nil {
		return reports.ECADReport{}, err
	}
	return reports.BuildECAD(entries, from, to, s.clock.Now()), nil
}

// BroadcastProof builds the chronological broadcast proof over [from, to). It
// reads the window's entries from the append-only log and projects them through
// reports.BuildBroadcastProof, which keeps only aired plays (Broadcasted=true,
// so DRM Spotify is excluded) and lists them in play order — no aggregation.
// GeneratedAt comes from the injected clock. Returns inbound.ErrBadRange when
// from is not strictly before to.
func (s *ReportService) BroadcastProof(ctx context.Context, from, to time.Time) (reports.BroadcastProof, error) {
	if !from.Before(to) {
		return reports.BroadcastProof{}, inbound.ErrBadRange
	}
	entries, err := s.repo.Since(ctx, from, to)
	if err != nil {
		return reports.BroadcastProof{}, err
	}
	return reports.BuildBroadcastProof(entries, from, to, s.clock.Now()), nil
}

// compile-time: ReportService satisfies the inbound port.
var _ inbound.ReportService = (*ReportService)(nil)
