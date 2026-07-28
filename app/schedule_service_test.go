package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/domain/shared"
	"autodj/ports/outbound"
)

func okClock(id string) schedule.Clock {
	return schedule.Clock{
		ID:   id,
		Name: id,
		Slots: []schedule.Slot{
			{Kind: schedule.SlotMusic, Category: library.CategoryMusic, Count: 3},
		},
	}
}

func TestScheduleClocksAndSave(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, nil)
	ctx := context.Background()

	if list, err := svc.Clocks(ctx); err != nil || len(list) != 0 {
		t.Fatalf("Clocks empty = %v, %v", list, err)
	}
	got, err := svc.SaveClock(ctx, okClock("manha"))
	if err != nil || got.ID != "manha" || len(got.Slots) != 1 {
		t.Fatalf("SaveClock = %+v, %v", got, err)
	}
	list, err := svc.Clocks(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("Clocks = %v, %v", list, err)
	}
}

func TestScheduleSaveInvalidNotPersisted(t *testing.T) {
	repo := memory.NewClockRepo()
	svc := NewScheduleService(repo, memory.NewGridRepo(), nil, nil)
	bad := okClock("x")
	bad.Slots[0].Count = 0 // invalid
	if _, err := svc.SaveClock(context.Background(), bad); !errors.Is(err, schedule.ErrSlotCount) {
		t.Fatalf("err = %v, want ErrSlotCount", err)
	}
	if list, _ := repo.List(context.Background()); len(list) != 0 {
		t.Fatal("invalid clock must not be persisted")
	}
}

func TestScheduleDeleteMissing(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, nil)
	if err := svc.DeleteClock(context.Background(), "nope"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("Delete missing = %v", err)
	}
}

func TestScheduleDeleteOK(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, nil)
	ctx := context.Background()
	if _, err := svc.SaveClock(ctx, okClock("a")); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteClock(ctx, "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if list, _ := svc.Clocks(ctx); len(list) != 0 {
		t.Fatal("clock not deleted")
	}
}

// failClockRepo forces Save/List/Get errors for the error-path branches.
type failClockRepo struct {
	saveErr, getErr, listErr error
}

func (r failClockRepo) List(context.Context) ([]schedule.Clock, error) {
	return nil, r.listErr
}
func (r failClockRepo) Get(context.Context, string) (schedule.Clock, error) {
	return schedule.Clock{}, r.getErr
}
func (r failClockRepo) Save(context.Context, schedule.Clock) error { return r.saveErr }
func (r failClockRepo) Delete(context.Context, string) error       { return nil }

func TestScheduleSaveRepoError(t *testing.T) {
	svc := NewScheduleService(failClockRepo{saveErr: errors.New("boom")}, memory.NewGridRepo(), nil, nil)
	if _, err := svc.SaveClock(context.Background(), okClock("a")); err == nil {
		t.Fatal("expected repo save error")
	}
}

func TestScheduleSaveGetError(t *testing.T) {
	svc := NewScheduleService(failClockRepo{getErr: errors.New("boom")}, memory.NewGridRepo(), nil, nil)
	if _, err := svc.SaveClock(context.Background(), okClock("a")); err == nil {
		t.Fatal("expected repo get error")
	}
}

// --- Generation (Story 4.3) ---

// localMusic builds a validated local (PCM, mixable) music track.
func localMusic(id, artist, title string, bpm float64) *library.Track {
	t, err := library.NewTrack(id, shared.SourceLocal, title, time.Minute)
	if err != nil {
		panic(err)
	}
	t.Artist = artist
	t.Category = string(library.CategoryMusic)
	t.BPM = bpm
	t.Path = "/music/" + id + ".flac" // local source requires a path
	return t
}

// commercialTrack builds a validated local (PCM, mixable) commercial track.
func commercialTrack(id string) *library.Track {
	t, err := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
	if err != nil {
		panic(err)
	}
	t.Category = string(library.CategoryCommercial)
	t.Path = "/spots/" + id + ".flac"
	return t
}

func TestGeneratePlaylistOrdersAndCounts(t *testing.T) {
	ctx := context.Background()
	tracks := memory.New()
	for _, tr := range []*library.Track{
		localMusic("t1", "A", "s1", 120),
		localMusic("t2", "B", "s2", 122),
		localMusic("t3", "C", "s3", 121),
	} {
		if err := tracks.Upsert(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	clocks := memory.NewClockRepo()
	if err := clocks.Save(ctx, okClock("manha")); err != nil { // one music slot, count 3
		t.Fatal(err)
	}
	svc := NewScheduleService(clocks, memory.NewGridRepo(), tracks, nil)

	list, err := svc.GeneratePlaylist(ctx, "manha")
	if err != nil {
		t.Fatalf("GeneratePlaylist: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	for _, tr := range list {
		if tr.ID == "" || !tr.Mixable {
			t.Fatalf("resolved track invalid: %+v", tr)
		}
	}
}

func TestGeneratePlaylistExcludesSpotify(t *testing.T) {
	ctx := context.Background()
	tracks := memory.New()
	// only a Spotify (non-PCM, non-mixable) music track exists → generation
	// finds no eligible candidate → empty catalog.
	sp, err := library.NewTrack("sp", shared.SourceSpotify, "drm", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sp.Artist = "A"
	sp.SpotifyURI = "spotify:track:x"
	sp.Category = string(library.CategoryMusic)
	if err := tracks.Upsert(ctx, sp); err != nil {
		t.Fatal(err)
	}
	clocks := memory.NewClockRepo()
	if err := clocks.Save(ctx, okClock("manha")); err != nil {
		t.Fatal(err)
	}
	svc := NewScheduleService(clocks, memory.NewGridRepo(), tracks, nil)

	if _, err := svc.GeneratePlaylist(ctx, "manha"); !errors.Is(err, schedule.ErrEmptyCategory) {
		t.Fatalf("err = %v, want ErrEmptyCategory (Spotify excluded)", err)
	}
}

func TestGeneratePlaylistUnknownClock(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), memory.New(), nil)
	if _, err := svc.GeneratePlaylist(context.Background(), "ghost"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("err = %v, want ErrClockNotFound", err)
	}
}

func TestGeneratePlaylistTrackListError(t *testing.T) {
	ctx := context.Background()
	clocks := memory.NewClockRepo()
	if err := clocks.Save(ctx, okClock("manha")); err != nil {
		t.Fatal(err)
	}
	svc := NewScheduleService(clocks, memory.NewGridRepo(), failTrackRepo{}, nil)
	if _, err := svc.GeneratePlaylist(ctx, "manha"); err == nil {
		t.Fatal("expected track list error")
	}
}

// failTrackRepo forces List to error (other methods unused here).
type failTrackRepo struct{}

func (failTrackRepo) Upsert(context.Context, *library.Track) error { return nil }
func (failTrackRepo) List(context.Context) ([]*library.Track, error) {
	return nil, errors.New("boom")
}
func (failTrackRepo) Get(context.Context, string) (*library.Track, error) { return nil, nil }
func (failTrackRepo) Count(context.Context) (int, error)                  { return 0, nil }
func (failTrackRepo) DeleteAll(context.Context) error                    { return nil }
func (failTrackRepo) Delete(context.Context, string) error               { return nil }

// --- Grid (Story 4.2) ---

func gridOf(cells ...schedule.Cell) schedule.Grid { return schedule.Grid{Cells: cells} }

func TestScheduleGridEmptyThenSave(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, nil)
	ctx := context.Background()

	if g, err := svc.Grid(ctx); err != nil || len(g.Cells) != 0 {
		t.Fatalf("Grid empty = %+v, %v", g, err)
	}
	// clock must exist so the grid validates.
	if _, err := svc.SaveClock(ctx, okClock("manha")); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.SaveGrid(ctx, gridOf(schedule.Cell{Weekday: time.Monday, Hour: 6, ClockID: "manha"}))
	if err != nil || len(saved.Cells) != 1 {
		t.Fatalf("SaveGrid = %+v, %v", saved, err)
	}
	got, err := svc.Grid(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := got.ClockAt(time.Monday, 6); !ok || id != "manha" {
		t.Fatalf("ClockAt = %q,%v", id, ok)
	}
}

func TestScheduleSaveGridUnknownClock(t *testing.T) {
	repo := memory.NewGridRepo()
	svc := NewScheduleService(memory.NewClockRepo(), repo, nil, nil)
	// no clocks known → cell referencing "ghost" is invalid.
	if _, err := svc.SaveGrid(context.Background(),
		gridOf(schedule.Cell{Weekday: time.Monday, Hour: 6, ClockID: "ghost"})); !errors.Is(err, schedule.ErrUnknownClock) {
		t.Fatalf("err = %v, want ErrUnknownClock", err)
	}
	if g, _ := repo.Get(context.Background()); len(g.Cells) != 0 {
		t.Fatal("invalid grid must not be persisted")
	}
}

func TestScheduleSaveGridKnownClocksError(t *testing.T) {
	svc := NewScheduleService(failClockRepo{listErr: errors.New("boom")}, memory.NewGridRepo(), nil, nil)
	if _, err := svc.SaveGrid(context.Background(),
		gridOf(schedule.Cell{Weekday: time.Monday, Hour: 6, ClockID: "manha"})); err == nil {
		t.Fatal("expected knownClocks list error")
	}
}

// failGridRepo forces Save/Get errors on the grid path.
type failGridRepo struct {
	saveErr, getErr error
	g               schedule.Grid
}

func (r *failGridRepo) Get(context.Context) (schedule.Grid, error) { return r.g, r.getErr }
func (r *failGridRepo) Save(_ context.Context, g schedule.Grid) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.g = g
	return nil
}

func TestScheduleSaveGridRepoError(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), &failGridRepo{saveErr: errors.New("boom")}, nil, nil)
	if _, err := svc.SaveGrid(context.Background(), schedule.Grid{}); err == nil {
		t.Fatal("expected grid save error")
	}
}

// --- Events (Story 4.4) ---

func okEvent(id string, prio int, ref string) schedule.ScheduledEvent {
	return schedule.ScheduledEvent{
		ID:       id,
		Kind:     schedule.EventComercial,
		Days:     []time.Weekday{time.Monday},
		Hour:     8,
		Minute:   0,
		Priority: prio,
		MediaRef: ref,
	}
}

func TestScheduleEventsCRUD(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, memory.NewEventRepo())
	ctx := context.Background()

	if list, err := svc.Events(ctx); err != nil || len(list) != 0 {
		t.Fatalf("Events empty = %v, %v", list, err)
	}
	got, err := svc.SaveEvent(ctx, okEvent("a", 50, "commercial"))
	if err != nil || got.ID != "a" || got.Priority != 50 {
		t.Fatalf("SaveEvent = %+v, %v", got, err)
	}
	if list, err := svc.Events(ctx); err != nil || len(list) != 1 {
		t.Fatalf("Events = %v, %v", list, err)
	}
	if err := svc.DeleteEvent(ctx, "a"); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if list, _ := svc.Events(ctx); len(list) != 0 {
		t.Fatal("event not deleted")
	}
}

func TestScheduleSaveEventInvalidNotPersisted(t *testing.T) {
	repo := memory.NewEventRepo()
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, repo)
	bad := okEvent("x", 50, "")
	bad.Hour = 99 // invalid
	if _, err := svc.SaveEvent(context.Background(), bad); !errors.Is(err, schedule.ErrEventHour) {
		t.Fatalf("err = %v, want ErrEventHour", err)
	}
	if list, _ := repo.List(context.Background()); len(list) != 0 {
		t.Fatal("invalid event must not be persisted")
	}
}

func TestScheduleDeleteEventMissing(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, memory.NewEventRepo())
	if err := svc.DeleteEvent(context.Background(), "nope"); !errors.Is(err, outbound.ErrEventNotFound) {
		t.Fatalf("Delete missing = %v", err)
	}
}

// failEventRepo forces List/Get/Save errors for the error-path branches.
type failEventRepo struct {
	listErr, getErr, saveErr error
}

func (r failEventRepo) List(context.Context) ([]schedule.ScheduledEvent, error) {
	return nil, r.listErr
}
func (r failEventRepo) Get(context.Context, string) (schedule.ScheduledEvent, error) {
	return schedule.ScheduledEvent{}, r.getErr
}
func (r failEventRepo) Save(context.Context, schedule.ScheduledEvent) error { return r.saveErr }
func (r failEventRepo) Delete(context.Context, string) error               { return nil }

func TestScheduleSaveEventRepoError(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, failEventRepo{saveErr: errors.New("boom")})
	if _, err := svc.SaveEvent(context.Background(), okEvent("a", 50, "")); err == nil {
		t.Fatal("expected event save error")
	}
}

func TestScheduleSaveEventGetError(t *testing.T) {
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), nil, failEventRepo{getErr: errors.New("boom")})
	if _, err := svc.SaveEvent(context.Background(), okEvent("a", 50, "")); err == nil {
		t.Fatal("expected event get error")
	}
}

// dueAt builds an instant at the given weekday/hour/minute (2024-01-01 is a Monday).
func dueMonday(hour, min int) time.Time {
	return time.Date(2024, 1, 1, hour, min, 0, 0, time.UTC)
}

func TestNextInjectionsResolvesByIDAndCategory(t *testing.T) {
	ctx := context.Background()
	tracks := memory.New()
	// commercial category tracks: c1 (first by id), c2. music: m1.
	for _, tr := range []*library.Track{
		commercialTrack("c2"),
		commercialTrack("c1"),
		localMusic("m1", "A", "s", 120),
	} {
		if err := tracks.Upsert(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	events := memory.NewEventRepo()
	// high prio resolves by exact id (m1); low prio resolves by category (commercial→c1).
	byID := okEvent("byid", 90, "m1")
	byCat := okEvent("bycat", 10, "commercial")
	if err := events.Save(ctx, byID); err != nil {
		t.Fatal(err)
	}
	if err := events.Save(ctx, byCat); err != nil {
		t.Fatal(err)
	}
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), tracks, events)

	out, err := svc.NextInjections(ctx, dueMonday(8, 0))
	if err != nil {
		t.Fatalf("NextInjections: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2 (%+v)", len(out), out)
	}
	// priority-first: byID (90) before byCat (10).
	if out[0].ID != "m1" {
		t.Fatalf("out[0] = %q, want m1 (higher priority)", out[0].ID)
	}
	if out[1].ID != "c1" {
		t.Fatalf("out[1] = %q, want c1 (first in commercial category)", out[1].ID)
	}
}

func TestNextInjectionsSkipsEmptyAndUnresolvable(t *testing.T) {
	ctx := context.Background()
	tracks := memory.New()
	events := memory.NewEventRepo()
	empty := okEvent("empty", 50, "")     // no media yet (TTS) → skipped
	ghost := okEvent("ghost", 40, "nope") // unresolvable ref → skipped
	for _, e := range []schedule.ScheduledEvent{empty, ghost} {
		if err := events.Save(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), tracks, events)
	out, err := svc.NextInjections(ctx, dueMonday(8, 0))
	if err != nil {
		t.Fatalf("NextInjections: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("len = %d, want 0 (%+v)", len(out), out)
	}
}

func TestNextInjectionsExcludesSpotify(t *testing.T) {
	ctx := context.Background()
	tracks := memory.New()
	sp, err := library.NewTrack("sp", shared.SourceSpotify, "drm", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sp.Artist = "A"
	sp.SpotifyURI = "spotify:track:x"
	sp.Category = string(library.CategoryCommercial)
	if err := tracks.Upsert(ctx, sp); err != nil {
		t.Fatal(err)
	}
	events := memory.NewEventRepo()
	// event points at the Spotify track by id AND its category — neither resolves.
	if err := events.Save(ctx, okEvent("e1", 50, "sp")); err != nil {
		t.Fatal(err)
	}
	if err := events.Save(ctx, okEvent("e2", 40, "commercial")); err != nil {
		t.Fatal(err)
	}
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), tracks, events)
	out, err := svc.NextInjections(ctx, dueMonday(8, 0))
	if err != nil {
		t.Fatalf("NextInjections: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("Spotify must never inject: got %+v", out)
	}
}

func TestNextInjectionsNoneDue(t *testing.T) {
	ctx := context.Background()
	events := memory.NewEventRepo()
	if err := events.Save(ctx, okEvent("a", 50, "commercial")); err != nil {
		t.Fatal(err)
	}
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), memory.New(), events)
	// event is Monday 08:00; ask at 09:00 → none due.
	out, err := svc.NextInjections(ctx, dueMonday(9, 0))
	if err != nil {
		t.Fatalf("NextInjections: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("len = %d, want 0", len(out))
	}
}

func TestNextInjectionsListErrors(t *testing.T) {
	ctx := context.Background()
	// events.List error
	svc := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), memory.New(), failEventRepo{listErr: errors.New("boom")})
	if _, err := svc.NextInjections(ctx, dueMonday(8, 0)); err == nil {
		t.Fatal("expected events list error")
	}
	// tracks.List error (event is due, so it reaches the track list)
	events := memory.NewEventRepo()
	if err := events.Save(ctx, okEvent("a", 50, "commercial")); err != nil {
		t.Fatal(err)
	}
	svc2 := NewScheduleService(memory.NewClockRepo(), memory.NewGridRepo(), failTrackRepo{}, events)
	if _, err := svc2.NextInjections(ctx, dueMonday(8, 0)); err == nil {
		t.Fatal("expected tracks list error")
	}
}
