package memory

import (
	"context"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/playlog"
	"autodj/domain/shared"
)

func plEntry(t *testing.T, id string, src shared.SourceKind, at time.Time) playlog.Entry {
	t.Helper()
	e, err := playlog.NewEntry(
		library.Track{ID: id, Title: "T-" + id, Artist: "A-" + id, Source: src, Duration: time.Minute}, at)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return e
}

func TestPlayLogRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewPlayLogRepo()
	base := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)

	// append out of chronological order; Since must return oldest first
	late := plEntry(t, "hot", shared.SourceLocal, base.Add(3*time.Hour))
	early := plEntry(t, "hot", shared.SourceLocal, base.Add(1*time.Hour))
	cold := plEntry(t, "cold", shared.SourceLocal, base.Add(2*time.Hour))
	for _, e := range []playlog.Entry{late, early, cold} {
		if err := r.Append(ctx, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// idempotent on ID
	if err := r.Append(ctx, late); err != nil {
		t.Fatalf("re-append: %v", err)
	}

	all, err := r.Since(ctx, base, base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("since len = %d, want 3 (idempotent)", len(all))
	}
	if !all[0].PlayedAt.Before(all[1].PlayedAt) || !all[1].PlayedAt.Before(all[2].PlayedAt) {
		t.Fatalf("since not oldest-first: %v", all)
	}

	// narrow window excludes out-of-range
	win, _ := r.Since(ctx, base.Add(90*time.Minute), base.Add(150*time.Minute))
	if len(win) != 1 || win[0].TrackID != "cold" {
		t.Fatalf("narrow window = %+v", win)
	}

	top, err := r.Top(ctx, base, base.Add(24*time.Hour), 10)
	if err != nil {
		t.Fatalf("top: %v", err)
	}
	if len(top) != 2 || top[0].TrackID != "hot" || top[0].Count != 2 {
		t.Fatalf("top = %+v, want hot/2 first", top)
	}
}
