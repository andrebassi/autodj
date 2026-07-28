package app

import (
	"context"
	"testing"
	"time"

	"autodj/adapters/driven/memory"
	"autodj/domain/library"
	"autodj/domain/schedule"
)

// fixedClock returns a constant instant so GradeLoad's "today" is deterministic.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// clean install (default): taxonomy always served, but the sample station
// content (folders, cartwall, program preview) is suppressed and the grade
// histogram is zeroed until a real grid is programmed.
func TestContentServiceClean(t *testing.T) {
	s := NewContentService()
	ctx := context.Background()

	op := s.Operational(ctx)
	if len(op.Categories) == 0 || len(op.MusicSubtabs) == 0 || len(op.Markers) == 0 {
		t.Fatalf("taxonomy must survive clean install: %+v", op)
	}
	if len(op.Folders) != 0 || len(op.Cartwall) != 0 {
		t.Fatalf("clean install must have empty folders/cartwall: %+v", op)
	}
	if tabs := s.WebTabs(ctx); len(tabs) == 0 {
		t.Fatalf("webtabs empty")
	}
	if pg := s.Program(ctx); len(pg.Above) != 0 || len(pg.Below) != 0 {
		t.Fatalf("clean install program must be empty: %+v", pg)
	}
	gl := s.GradeLoad(ctx)
	if len(gl.Hourly) != 24 {
		t.Fatalf("grade load must have 24 hours, got %d", len(gl.Hourly))
	}
	for h, v := range gl.Hourly {
		if v != 0 {
			t.Fatalf("clean install grade hour %d = %d, want 0", h, v)
		}
	}
}

// demo mode restores the canned catalogs (folders, cartwall, program, histogram).
func TestContentServiceDemo(t *testing.T) {
	s := NewContentService().WithDemo(true)
	ctx := context.Background()

	if op := s.Operational(ctx); len(op.Cartwall) == 0 || len(op.Folders) == 0 {
		t.Fatalf("demo mode must serve folders/cartwall: %+v", op)
	}
	if pg := s.Program(ctx); len(pg.Above) == 0 || len(pg.Below) == 0 {
		t.Fatalf("demo mode program empty: %+v", pg)
	}
	// no grade source → demo preview (non-zero occupancy).
	gl := s.GradeLoad(ctx)
	sum := 0
	for _, v := range gl.Hourly {
		sum += v
	}
	if sum == 0 {
		t.Fatalf("demo grade preview must be non-zero")
	}
}

// clockWith2 builds a clock whose slots sum to 2 items.
func clockWith2(id string) schedule.Clock {
	return schedule.Clock{
		ID:    id,
		Name:  id,
		Slots: []schedule.Slot{{Kind: schedule.SlotMusic, Category: library.CategoryMusic, Count: 2}},
	}
}

func TestGradeLoadFallbackWhenNoSource(t *testing.T) {
	s := NewContentService()
	if gl := s.GradeLoad(context.Background()); len(gl.Hourly) != 24 {
		t.Fatalf("fallback must have 24 hours, got %d", len(gl.Hourly))
	}
}

func TestGradeLoadFallbackWhenGridEmpty(t *testing.T) {
	s := NewContentService()
	s.SetGradeSource(memory.NewGridRepo(), memory.NewClockRepo(), fixedClock{time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)})
	// grid empty → mock preview (still 24h).
	if gl := s.GradeLoad(context.Background()); len(gl.Hourly) != 24 {
		t.Fatalf("empty-grid fallback must have 24 hours, got %d", len(gl.Hourly))
	}
}

func TestGradeLoadDerivedFromGrid(t *testing.T) {
	ctx := context.Background()
	clocks := memory.NewClockRepo()
	grid := memory.NewGridRepo()
	if err := clocks.Save(ctx, clockWith2("manha")); err != nil {
		t.Fatal(err)
	}
	// 2026-07-20 is a Monday. Program hour 9 with "manha" (2 items).
	if err := grid.Save(ctx, schedule.Grid{Cells: []schedule.Cell{
		{Weekday: time.Monday, Hour: 9, ClockID: "manha"},
	}}); err != nil {
		t.Fatal(err)
	}
	s := NewContentService()
	s.SetGradeSource(grid, clocks, fixedClock{time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)})

	gl := s.GradeLoad(ctx)
	if len(gl.Hourly) != 24 {
		t.Fatalf("derived must have 24 hours, got %d", len(gl.Hourly))
	}
	if gl.Hourly[9] != 2 {
		t.Fatalf("hour 9 load = %d, want 2", gl.Hourly[9])
	}
	if gl.Hourly[0] != 0 {
		t.Fatalf("unprogrammed hour 0 = %d, want 0", gl.Hourly[0])
	}
}

// failListClocks errors on List to exercise GradeLoad's clock-error fallback.
type failListClocks struct{}

func (failListClocks) List(context.Context) ([]schedule.Clock, error) {
	return nil, errListBoom
}
func (failListClocks) Get(context.Context, string) (schedule.Clock, error) {
	return schedule.Clock{}, nil
}
func (failListClocks) Save(context.Context, schedule.Clock) error { return nil }
func (failListClocks) Delete(context.Context, string) error       { return nil }

var errListBoom = listBoom{}

type listBoom struct{}

func (listBoom) Error() string { return "list boom" }

func TestGradeLoadFallbackOnClockError(t *testing.T) {
	ctx := context.Background()
	grid := memory.NewGridRepo()
	if err := grid.Save(ctx, schedule.Grid{Cells: []schedule.Cell{
		{Weekday: time.Monday, Hour: 9, ClockID: "manha"},
	}}); err != nil {
		t.Fatal(err)
	}
	s := NewContentService()
	s.SetGradeSource(grid, failListClocks{}, fixedClock{time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)})
	if gl := s.GradeLoad(ctx); len(gl.Hourly) != 24 {
		t.Fatalf("fallback must have 24 hours, got %d", len(gl.Hourly))
	}
}
