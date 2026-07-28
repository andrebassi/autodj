package app

import (
	"context"
	"time"

	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

// ScheduleService manages the station's programming templates: clocks (hour
// templates, Story 4.1), the weekly grid (Story 4.2) and rotation-based playlist
// generation (Story 4.3). It validates on write so an incoherent clock or grid
// never reaches storage. Events and autopilot land in later Épico 4 stories.
type ScheduleService struct {
	clocks   outbound.ClockRepo
	grid     outbound.GridRepo
	tracks   outbound.TrackRepo
	events   outbound.EventRepo
	rotation schedule.RotationRules
}

// NewScheduleService builds the schedule service over a clock repo, a grid repo,
// the catalog track repo and the events repo. grid may be nil in setups that only
// use clocks (Grid/SaveGrid then error); tracks may be nil where generation is
// unused (GeneratePlaylist/NextInjections then error); events may be nil where
// scheduled events are unused (Events/SaveEvent/… then panic on the nil repo —
// wire a repo whenever events are exercised). Generation uses the default
// rotation rules.
func NewScheduleService(clocks outbound.ClockRepo, grid outbound.GridRepo, tracks outbound.TrackRepo, events outbound.EventRepo) *ScheduleService {
	return &ScheduleService{clocks: clocks, grid: grid, tracks: tracks, events: events, rotation: schedule.DefaultRotation()}
}

// Clocks lists the station's hour templates (ordered by id in the repo).
func (s *ScheduleService) Clocks(ctx context.Context) ([]schedule.Clock, error) {
	return s.clocks.List(ctx)
}

// SaveClock validates then upserts the clock, returning the stored value. On a
// validation error the store is left untouched.
func (s *ScheduleService) SaveClock(ctx context.Context, c schedule.Clock) (schedule.Clock, error) {
	if err := c.Validate(); err != nil {
		return schedule.Clock{}, err
	}
	if err := s.clocks.Save(ctx, c); err != nil {
		return schedule.Clock{}, err
	}
	return s.clocks.Get(ctx, c.ID)
}

// DeleteClock removes a clock by id. Returns outbound.ErrClockNotFound when the
// id is unknown.
func (s *ScheduleService) DeleteClock(ctx context.Context, id string) error {
	return s.clocks.Delete(ctx, id)
}

// Grid returns the station's weekly grid (empty when never saved).
func (s *ScheduleService) Grid(ctx context.Context) (schedule.Grid, error) {
	return s.grid.Get(ctx)
}

// SaveGrid validates the grid against the currently-known clocks, then persists
// it and returns the stored value. On a validation error the store is untouched.
func (s *ScheduleService) SaveGrid(ctx context.Context, g schedule.Grid) (schedule.Grid, error) {
	known, err := s.knownClocks(ctx)
	if err != nil {
		return schedule.Grid{}, err
	}
	if err := g.Validate(known); err != nil {
		return schedule.Grid{}, err
	}
	if err := s.grid.Save(ctx, g); err != nil {
		return schedule.Grid{}, err
	}
	return s.grid.Get(ctx)
}

// GeneratePlaylist materializes the clock into a concrete, ordered playlist of
// tracks obeying the rotation rules (artist/title separation, BPM smoothing). It
// resolves the clock, projects the eligible catalog (Mixable && PCMAvailable —
// the two-engine invariant keeps Spotify out of generation) into per-category
// candidates, and calls the pure schedule.Generate. Recent history is empty for
// now (it will come from the PlayLog in Épico 7). Unknown clock →
// outbound.ErrClockNotFound; a catalog that yields nothing → ErrEmptyCategory.
func (s *ScheduleService) GeneratePlaylist(ctx context.Context, clockID string) ([]library.Track, error) {
	clock, err := s.clocks.Get(ctx, clockID)
	if err != nil {
		return nil, err
	}
	tracks, err := s.tracks.List(ctx)
	if err != nil {
		return nil, err
	}
	pool := make(map[library.Category][]schedule.Candidate)
	byID := make(map[string]*library.Track, len(tracks))
	for _, t := range tracks {
		if t == nil || !t.Mixable || !t.PCMAvailable {
			continue // Spotify/DRM and non-PCM never enter generation
		}
		cat := library.Category(t.Category)
		pool[cat] = append(pool[cat], schedule.Candidate{
			TrackID:  t.ID,
			Artist:   t.Artist,
			Title:    t.Title,
			BPM:      t.BPM,
			Category: cat,
		})
		byID[t.ID] = t
	}
	ids, err := schedule.Generate(clock, pool, nil, s.rotation)
	if err != nil {
		return nil, err
	}
	out := make([]library.Track, 0, len(ids))
	for _, id := range ids {
		if t := byID[id]; t != nil {
			out = append(out, *t)
		}
	}
	return out, nil
}

// Events lists the station's scheduled events (id-ordered in the repo).
func (s *ScheduleService) Events(ctx context.Context) ([]schedule.ScheduledEvent, error) {
	return s.events.List(ctx)
}

// SaveEvent validates then upserts the event, returning the stored value. On a
// validation error the store is left untouched.
func (s *ScheduleService) SaveEvent(ctx context.Context, e schedule.ScheduledEvent) (schedule.ScheduledEvent, error) {
	if err := e.Validate(); err != nil {
		return schedule.ScheduledEvent{}, err
	}
	if err := s.events.Save(ctx, e); err != nil {
		return schedule.ScheduledEvent{}, err
	}
	return s.events.Get(ctx, e.ID)
}

// DeleteEvent removes an event by id. Returns outbound.ErrEventNotFound when the
// id is unknown.
func (s *ScheduleService) DeleteEvent(ctx context.Context, id string) error {
	return s.events.Delete(ctx, id)
}

// NextInjections resolves the events due at instant at into the media to inject,
// priority-first (preemption order). It lists the events, filters those due
// (schedule.DueEvents — already sorted priority-desc), and resolves each event's
// MediaRef to a local, mixable/PCM track. MediaRef resolves by exact track id
// first, else as a media category (first eligible track in that category, id
// order). Events with an empty MediaRef (media generated later, e.g. TTS) or an
// unresolvable ref are skipped. Spotify/DRM is never eligible — the two-engine
// invariant holds for injections too.
func (s *ScheduleService) NextInjections(ctx context.Context, at time.Time) ([]library.Track, error) {
	events, err := s.events.List(ctx)
	if err != nil {
		return nil, err
	}
	due := schedule.DueEvents(events, at)
	if len(due) == 0 {
		return []library.Track{}, nil
	}
	tracks, err := s.tracks.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*library.Track, len(tracks))
	byCat := make(map[string]*library.Track)
	for _, t := range tracks {
		if t == nil || !t.Mixable || !t.PCMAvailable {
			continue // Spotify/DRM and non-PCM never resolve an injection
		}
		byID[t.ID] = t
		if cur, ok := byCat[t.Category]; !ok || t.ID < cur.ID {
			byCat[t.Category] = t // first eligible in category, id order
		}
	}
	out := make([]library.Track, 0, len(due))
	for _, e := range due {
		if e.MediaRef == "" {
			continue // generated later (TTS); nothing to inject yet
		}
		if t := byID[e.MediaRef]; t != nil {
			out = append(out, *t)
			continue
		}
		if t := byCat[e.MediaRef]; t != nil {
			out = append(out, *t)
		}
	}
	return out, nil
}

// knownClocks builds the id→exists set the grid validates against.
func (s *ScheduleService) knownClocks(ctx context.Context) (map[string]bool, error) {
	list, err := s.clocks.List(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(list))
	for _, c := range list {
		known[c.ID] = true
	}
	return known, nil
}
