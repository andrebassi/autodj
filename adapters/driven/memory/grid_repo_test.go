package memory

import (
	"context"
	"testing"
	"time"

	"autodj/domain/schedule"
)

func TestGridRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewGridRepo()

	// starts empty.
	g, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(g.Cells) != 0 {
		t.Fatalf("fresh grid cells = %d, want 0", len(g.Cells))
	}

	// save then read back.
	want := schedule.Grid{Cells: []schedule.Cell{
		{Weekday: time.Monday, Hour: 6, ClockID: "manha"},
	}}
	if err := r.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("Get after save: %v", err)
	}
	if len(got.Cells) != 1 || got.Cells[0].ClockID != "manha" {
		t.Fatalf("round-trip grid = %+v", got)
	}
}
