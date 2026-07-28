package app

import (
	"context"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/library"
	"autodj/domain/playback"
	"autodj/domain/shared"
)

func TestPlayLogRecorderRecordsOnChange(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repo := memory.New()
	for _, id := range []string{"a", "b"} {
		if err := func() error { tk := trk(id, shared.SourceLocal); tk.Path = "/m/" + id + ".flac"; return repo.Upsert(ctx, &tk) }(); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}
	plRepo := memory.NewPlayLogRepo()
	svc := NewPlayLogService(plRepo, fixedClock{now})
	rec := NewPlayLogRecorder(svc, repo, fixedClock{now})

	// empty TrackID → no-op
	rec.Observe(ctx, playback.Playable{})
	// first track → recorded
	rec.Observe(ctx, playback.Playable{TrackID: "a", Source: shared.SourceLocal})
	// repeat poll of same track → deduped
	rec.Observe(ctx, playback.Playable{TrackID: "a", Source: shared.SourceLocal})
	// track change → recorded
	rec.Observe(ctx, playback.Playable{TrackID: "b", Source: shared.SourceLocal})
	// unknown track → repo.Get fails, swallowed (still advances last)
	rec.Observe(ctx, playback.Playable{TrackID: "ghost"})

	hist, err := svc.History(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2 (a,b deduped), got %+v", len(hist), hist)
	}
	ids := map[string]bool{}
	for _, e := range hist {
		ids[e.TrackID] = true
		if e.Artist == "" {
			t.Fatalf("entry %s lost artist (not resolved from catalog)", e.TrackID)
		}
	}
	if !ids["a"] || !ids["b"] {
		t.Fatalf("missing a/b: %+v", ids)
	}
}

func TestPlayLogRecorderResolvesFullTrack(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	repo := memory.New()
	full := library.Track{ID: "x", Title: "Aurora", Artist: "Nébula", Source: shared.SourceLocal, Path: "/m/x.flac", Duration: 3 * time.Minute}
	if err := repo.Upsert(ctx, &full); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	plRepo := memory.NewPlayLogRepo()
	svc := NewPlayLogService(plRepo, fixedClock{now})
	rec := NewPlayLogRecorder(svc, repo, fixedClock{now})

	// the Playable feed lacks Artist; the recorder must resolve it from the catalog
	rec.Observe(ctx, playback.Playable{TrackID: "x", Source: shared.SourceLocal, Title: "Aurora"})

	hist, _ := svc.History(ctx, now.Add(-time.Hour), now.Add(time.Hour))
	if len(hist) != 1 || hist[0].Artist != "Nébula" || hist[0].DurationMs != (3 * time.Minute).Milliseconds() {
		t.Fatalf("full-track resolve wrong: %+v", hist)
	}
}
