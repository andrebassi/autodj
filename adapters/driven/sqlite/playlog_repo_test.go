package sqlite

import (
	"context"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/playlog"
	"autodj/domain/shared"
)

func newPlayLogRepo(t *testing.T) *PlayLogRepo {
	t.Helper()
	tr, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = closeFn() })
	return tr.PlayLog()
}

func plEntry(t *testing.T, id string, src shared.SourceKind, at time.Time) playlog.Entry {
	t.Helper()
	e, err := playlog.NewEntry(
		library.Track{ID: id, Title: "T-" + id, Artist: "A-" + id, Source: src, Duration: time.Minute}, at)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return e
}

func TestPlayLogRepoSQLite(t *testing.T) {
	ctx := context.Background()
	r := newPlayLogRepo(t)
	base := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)

	// spotify entry: broadcasted/ecad both false round-trip
	if err := r.Append(ctx, plEntry(t, "sp", shared.SourceSpotify, base)); err != nil {
		t.Fatalf("append spotify: %v", err)
	}
	// two local plays of "hot", one of "cold"
	hotA := plEntry(t, "hot", shared.SourceLocal, base.Add(1*time.Hour))
	hotB := plEntry(t, "hot", shared.SourceLocal, base.Add(2*time.Hour))
	cold := plEntry(t, "cold", shared.SourceLocal, base.Add(3*time.Hour))
	for _, e := range []playlog.Entry{hotA, hotB, cold} {
		if err := r.Append(ctx, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// idempotent: re-append hotA is a no-op
	if err := r.Append(ctx, hotA); err != nil {
		t.Fatalf("re-append: %v", err)
	}

	// Since covers the whole day, oldest first
	all, err := r.Since(ctx, base, base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("since len = %d, want 4 (idempotent)", len(all))
	}
	if all[0].TrackID != "sp" || all[0].Broadcasted || all[0].ECADRelevant {
		t.Fatalf("spotify round-trip wrong: %+v", all[0])
	}
	if !all[1].Broadcasted || !all[1].ECADRelevant {
		t.Fatalf("local not broadcast/ECAD: %+v", all[1])
	}
	if all[0].Source != shared.SourceSpotify {
		t.Fatalf("source round-trip = %q", all[0].Source)
	}

	// Since window excludes out-of-range
	win, _ := r.Since(ctx, base.Add(90*time.Minute), base.Add(150*time.Minute))
	if len(win) != 1 || win[0].TrackID != "hot" {
		t.Fatalf("narrow window = %+v", win)
	}

	// Top ranks hot above cold
	top, err := r.Top(ctx, base, base.Add(24*time.Hour), 10)
	if err != nil {
		t.Fatalf("top: %v", err)
	}
	if len(top) != 3 || top[0].TrackID != "hot" || top[0].Count != 2 {
		t.Fatalf("top = %+v, want hot/2 first", top)
	}
	if top[0].Title != "T-hot" {
		t.Fatalf("top title = %q", top[0].Title)
	}

	// Top with n<=0 yields nil
	if got, _ := r.Top(ctx, base, base.Add(24*time.Hour), 0); got != nil {
		t.Fatalf("n=0 top = %+v, want nil", got)
	}
	// Top limit truncates
	lim, _ := r.Top(ctx, base, base.Add(24*time.Hour), 1)
	if len(lim) != 1 {
		t.Fatalf("limit top len = %d, want 1", len(lim))
	}
}
