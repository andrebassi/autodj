package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/playback"
	"autodj/domain/playlog"
	"autodj/domain/shared"
)

// --- fake PlaybackService (only NowPlaying/Queue matter here) ---

type fakePlayback struct {
	now      playback.Playable
	upcoming []playback.QueueItem
	err      error
}

func (f *fakePlayback) LoadFromLibrary(context.Context) (int, error) { return 0, nil }
func (f *fakePlayback) Enqueue(context.Context, string, int, bool, string) error { return nil }
func (f *fakePlayback) Dequeue(context.Context, int) error                        { return nil }
func (f *fakePlayback) Move(context.Context, int, int) error                       { return nil }
func (f *fakePlayback) Next(context.Context) error                                { return nil }
func (f *fakePlayback) Clear(context.Context) error                               { return nil }
func (f *fakePlayback) NowPlaying(context.Context) (playback.Playable, []playback.QueueItem, error) {
	return f.now, f.upcoming, f.err
}
func (f *fakePlayback) Queue(context.Context) []playback.QueueItem { return f.upcoming }

func psLocal(id string) playback.Playable {
	return playback.Playable{TrackID: id, Source: shared.SourceLocal, Title: "T-" + id, URI: "/" + id}
}

func psEntry(id string, at time.Time) playlog.Entry {
	e, _ := playlog.NewEntry(library.Track{ID: id, Title: "T-" + id, Artist: "A-" + id, Source: shared.SourceLocal}, at)
	return e
}

func TestPublicSiteSnapshotOK(t *testing.T) {
	ctx := context.Background()
	gen := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	play := &fakePlayback{
		now:      psLocal("cur"),
		upcoming: []playback.QueueItem{{Playable: psLocal("nxt"), Transition: shared.TransitionCrossfade}},
	}
	// two entries inside the 24h window, oldest first
	repo := &memPlayLogRepo{entries: []playlog.Entry{
		psEntry("a", gen.Add(-2*time.Hour)),
		psEntry("b", gen.Add(-1*time.Hour)),
	}}
	svc := NewPublicSiteService(play, repo, fixedClock{gen})

	snap, err := svc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !snap.NowPlaying.OnAir || snap.NowPlaying.TrackID != "cur" {
		t.Fatalf("now: %+v", snap.NowPlaying)
	}
	if len(snap.Upcoming) != 1 || snap.Upcoming[0].TrackID != "nxt" {
		t.Fatalf("upcoming: %+v", snap.Upcoming)
	}
	// newest-first
	if len(snap.Recent) != 2 || snap.Recent[0].TrackID != "b" || snap.Recent[1].TrackID != "a" {
		t.Fatalf("recent: %+v", snap.Recent)
	}
	if !snap.GeneratedAt.Equal(gen) {
		t.Fatalf("generatedAt: %v", snap.GeneratedAt)
	}
	if snap.Daily.DayOfYear != gen.YearDay() {
		t.Fatalf("daily: %+v", snap.Daily)
	}
}

func TestPublicSiteSnapshotOldEntriesExcluded(t *testing.T) {
	ctx := context.Background()
	gen := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repo := &memPlayLogRepo{entries: []playlog.Entry{
		psEntry("old", gen.Add(-48*time.Hour)), // outside 24h window
		psEntry("new", gen.Add(-1*time.Hour)),
	}}
	svc := NewPublicSiteService(&fakePlayback{}, repo, fixedClock{gen})

	snap, err := svc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.Recent) != 1 || snap.Recent[0].TrackID != "new" {
		t.Fatalf("window filter: %+v", snap.Recent)
	}
	if snap.NowPlaying.OnAir {
		t.Fatalf("expected off-air: %+v", snap.NowPlaying)
	}
}

func TestPublicSiteSnapshotPlaybackError(t *testing.T) {
	boom := errors.New("playback boom")
	svc := NewPublicSiteService(&fakePlayback{err: boom}, &memPlayLogRepo{}, fixedClock{time.Now()})
	if _, err := svc.Snapshot(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("want playback error, got %v", err)
	}
}

func TestPublicSiteSnapshotRepoError(t *testing.T) {
	repo := &memPlayLogRepo{failOn: "Since"}
	svc := NewPublicSiteService(&fakePlayback{}, repo, fixedClock{time.Now()})
	if _, err := svc.Snapshot(context.Background()); !errors.Is(err, errPlBoom) {
		t.Fatalf("want repo error, got %v", err)
	}
}
