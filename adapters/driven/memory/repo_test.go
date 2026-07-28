package memory

import (
	"context"
	"testing"
	"time"

	"autodj/domain/library"
	"autodj/domain/shared"
)

func mk(t *testing.T, id string) *library.Track {
	t.Helper()
	tr, err := library.NewTrack(id, shared.SourceLocal, id, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tr.Path = "/" + id
	return tr
}

func TestUpsertGetListCount(t *testing.T) {
	ctx := context.Background()
	r := New()
	if n, _ := r.Count(ctx); n != 0 {
		t.Fatal("empty count")
	}
	if _, err := r.Get(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := r.Upsert(ctx, mk(t, "b")); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(ctx, mk(t, "a")); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, "a")
	if got.ID != "a" {
		t.Fatal("get a")
	}
	list, _ := r.List(ctx)
	if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("list must be sorted by id: %+v", list)
	}
	n, _ := r.Count(ctx)
	if n != 2 {
		t.Fatal("count 2")
	}
	// upsert same id replaces, not duplicates
	repl := mk(t, "a")
	repl.Title = "changed"
	_ = r.Upsert(ctx, repl)
	if n, _ := r.Count(ctx); n != 2 {
		t.Fatal("upsert must not duplicate")
	}
	g, _ := r.Get(ctx, "a")
	if g.Title != "changed" {
		t.Fatal("upsert must replace")
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	r := New()
	if err := r.Delete(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("delete missing want ErrNotFound, got %v", err)
	}
	_ = r.Upsert(ctx, mk(t, "x"))
	if err := r.Delete(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "x"); err != ErrNotFound {
		t.Fatal("track should be gone after delete")
	}
	if err := r.Delete(ctx, "x"); err != ErrNotFound {
		t.Fatal("second delete want ErrNotFound")
	}
}

// The memory repo must deep-copy the Markers slice: mutating a track after
// Upsert (or a returned Get) must not alias the stored copy's backing array.
func TestUpsertClonesMarkers(t *testing.T) {
	ctx := context.Background()
	r := New()
	tr := mk(t, "x")
	tr.Duration = 4 * time.Minute
	if err := tr.SetMarkers([]library.Marker{{Kind: library.MarkerIntro, PosMs: 5000}}); err != nil {
		t.Fatal(err)
	}
	_ = r.Upsert(ctx, tr)

	// Mutate the caller's slice after storing — stored copy must be unaffected.
	tr.Markers[0].PosMs = 99999
	tr.Markers = append(tr.Markers, library.Marker{Kind: library.MarkerEnd, PosMs: 240000})

	got, _ := r.Get(ctx, "x")
	if len(got.Markers) != 1 || got.Markers[0].PosMs != 5000 {
		t.Fatalf("stored markers must not alias caller slice: %+v", got.Markers)
	}
	// And mutating the returned copy must not affect a subsequent Get.
	got.Markers[0].PosMs = 123
	again, _ := r.Get(ctx, "x")
	if again.Markers[0].PosMs != 5000 {
		t.Fatalf("returned markers must not alias stored slice: %+v", again.Markers)
	}
}

func TestUpsertRejectsInvalid(t *testing.T) {
	bad := &library.Track{ID: "", Source: shared.SourceLocal, Duration: time.Minute}
	if err := New().Upsert(context.Background(), bad); err == nil {
		t.Fatal("invalid track must be rejected")
	}
}
