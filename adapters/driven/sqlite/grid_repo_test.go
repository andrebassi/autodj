package sqlite

import (
	"context"
	"testing"
	"time"

	"autodj/domain/schedule"
)

func gridFixture() schedule.Grid {
	return schedule.Grid{Cells: []schedule.Cell{
		{Weekday: time.Monday, Hour: 6, ClockID: "manha"},
		{Weekday: time.Monday, Hour: 13, ClockID: "tarde"},
	}}
}

func TestGridRepoSQLite(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	gr := repo.Grid()
	ctx := context.Background()

	// empty when never saved.
	if g, err := gr.Get(ctx); err != nil || len(g.Cells) != 0 {
		t.Fatalf("Get empty = %+v, %v", g, err)
	}

	// save then read back.
	if err := gr.Save(ctx, gridFixture()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	g, err := gr.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(g.Cells) != 2 {
		t.Fatalf("cells = %d, want 2", len(g.Cells))
	}
	if id, ok := g.ClockAt(time.Monday, 13); !ok || id != "tarde" {
		t.Fatalf("ClockAt = %q,%v", id, ok)
	}

	// save again replaces (single-row upsert).
	if err := gr.Save(ctx, schedule.Grid{Cells: []schedule.Cell{
		{Weekday: time.Sunday, Hour: 0, ClockID: "madrugada"},
	}}); err != nil {
		t.Fatalf("Save replace: %v", err)
	}
	g2, _ := gr.Get(ctx)
	if len(g2.Cells) != 1 || g2.Cells[0].ClockID != "madrugada" {
		t.Fatalf("replace grid = %+v", g2)
	}
}

func TestGridRepoSQLiteDecodeError(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if _, err := repo.db.Exec(`INSERT INTO grid (id, data) VALUES (1, ?)`, "{not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := repo.Grid().Get(context.Background()); err == nil {
		t.Fatal("expected decode error on Get")
	}
}

func TestGridRepoSQLiteClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	gr := repo.Grid()
	closeFn()
	ctx := context.Background()
	if _, err := gr.Get(ctx); err == nil {
		t.Fatal("expected Get error on closed db")
	}
	if err := gr.Save(ctx, gridFixture()); err == nil {
		t.Fatal("expected Save error on closed db")
	}
}
