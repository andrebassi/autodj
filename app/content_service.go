package app

import (
	"context"

	"autodj/domain/content"
	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

// ContentService serves the station's canned UI catalogs. Most of it is fixed
// domain content (no state), but GradeLoad is derived from the real weekly grid
// when a grade source is wired (Story 4.2): the per-hour histogram reflects the
// clocks assigned to today's hours, falling back to the mock preview when no
// source is wired or the grid is empty.
type ContentService struct {
	grid   outbound.GridRepo
	clocks outbound.ClockRepo
	clock  outbound.Clock
	demo   bool
}

// NewContentService builds the content service with no grade source: GradeLoad
// falls back to a zeroed histogram until SetGradeSource wires the real grid.
// Clean install by default (demo=false): the mock station content — sample
// folders, cartwall buttons and the fake program preview — is suppressed so the
// operator starts from a truly empty station. WithDemo(true) restores the seed
// content for local demos/screenshots.
func NewContentService() *ContentService { return &ContentService{} }

// WithDemo toggles the canned station content (folders / cartwall / program
// preview / grade histogram). Off = clean install (empty). On = demo catalogs.
func (s *ContentService) WithDemo(on bool) *ContentService { s.demo = on; return s }

// SetGradeSource wires the weekly grid, the clock catalog and a time source so
// GradeLoad derives the day's real per-hour occupancy. Pass all three; leaving
// any nil keeps the mock preview.
func (s *ContentService) SetGradeSource(grid outbound.GridRepo, clocks outbound.ClockRepo, clock outbound.Clock) {
	s.grid = grid
	s.clocks = clocks
	s.clock = clock
}

// Operational returns the Operacional-screen catalog. The taxonomy (categories,
// music subtabs, marker palette, default mix config) is always served; the
// sample station content (folders, cartwall) is served only in demo mode —
// clean install starts with empty folders/cartwall for the operator to fill.
func (s *ContentService) Operational(_ context.Context) content.Operational {
	op := content.OperationalCatalog()
	if !s.demo {
		op.Folders = []content.Folder{}
		op.Cartwall = []content.CartButton{}
	}
	return op
}

// WebTabs returns the Internet side-panel tab catalog.
func (s *ContentService) WebTabs(_ context.Context) []content.WebTab {
	return content.WebTabs()
}

// Program returns the program preview rows. Clean install serves an empty
// program (the list fills from the real queue/log as the station runs); demo
// mode serves the canned RadioPro-style preview.
func (s *ContentService) Program(_ context.Context) content.Program {
	if !s.demo {
		return content.Program{Above: []content.ProgramRow{}, Below: []content.ProgramRow{}}
	}
	return content.ProgramPreview()
}

// GradeLoad returns the per-hour grade occupancy (24h histogram source). With a
// grade source wired and the grid programmed, it derives the histogram from
// today's grid via schedule.HourlyLoad; otherwise it returns the mock preview
// (no source, load error, or empty grid).
func (s *ContentService) GradeLoad(ctx context.Context) content.GradeLoad {
	if s.grid == nil || s.clocks == nil || s.clock == nil {
		return s.gradeFallback()
	}
	grid, err := s.grid.Get(ctx)
	if err != nil || len(grid.Cells) == 0 {
		return s.gradeFallback()
	}
	list, err := s.clocks.List(ctx)
	if err != nil {
		return s.gradeFallback()
	}
	byID := make(map[string]schedule.Clock, len(list))
	for _, c := range list {
		byID[c.ID] = c
	}
	hourly := schedule.HourlyLoad(grid, byID, s.clock.Now().Weekday())
	return content.GradeLoad{Hourly: hourly[:]}
}

// gradeFallback is the histogram used when no real grid is available: the demo
// preview in demo mode, a zeroed 24h histogram on a clean install.
func (s *ContentService) gradeFallback() content.GradeLoad {
	if s.demo {
		return content.GradeLoadPreview()
	}
	return content.EmptyGradeLoad()
}
