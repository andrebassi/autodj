package schedule

import (
	"testing"
	"time"

	"autodj/domain/library"
)

func knownFromDefault() map[string]bool {
	k := map[string]bool{}
	for _, c := range DefaultClocks() {
		k[c.ID] = true
	}
	return k
}

func TestGridValidate(t *testing.T) {
	known := map[string]bool{"manha": true, "tarde": true}
	tests := []struct {
		name string
		grid Grid
		want error
	}{
		{"happy", Grid{Cells: []Cell{
			{Weekday: time.Monday, Hour: 6, ClockID: "manha"},
			{Weekday: time.Monday, Hour: 13, ClockID: "tarde"},
		}}, nil},
		{"empty grid ok", Grid{}, nil},
		{"bad weekday low", Grid{Cells: []Cell{{Weekday: -1, Hour: 6, ClockID: "manha"}}}, ErrBadWeekday},
		{"bad weekday high", Grid{Cells: []Cell{{Weekday: 7, Hour: 6, ClockID: "manha"}}}, ErrBadWeekday},
		{"bad hour low", Grid{Cells: []Cell{{Weekday: time.Monday, Hour: -1, ClockID: "manha"}}}, ErrBadHour},
		{"bad hour high", Grid{Cells: []Cell{{Weekday: time.Monday, Hour: 24, ClockID: "manha"}}}, ErrBadHour},
		{"duplicate", Grid{Cells: []Cell{
			{Weekday: time.Monday, Hour: 6, ClockID: "manha"},
			{Weekday: time.Monday, Hour: 6, ClockID: "tarde"},
		}}, ErrDuplicateCell},
		{"unknown clock", Grid{Cells: []Cell{{Weekday: time.Monday, Hour: 6, ClockID: "ghost"}}}, ErrUnknownClock},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.grid.Validate(known); err != tt.want {
				t.Fatalf("Validate = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestGridClockAt(t *testing.T) {
	g := Grid{Cells: []Cell{{Weekday: time.Tuesday, Hour: 9, ClockID: "manha"}}}
	if id, ok := g.ClockAt(time.Tuesday, 9); !ok || id != "manha" {
		t.Fatalf("ClockAt hit = %q,%v", id, ok)
	}
	if _, ok := g.ClockAt(time.Tuesday, 10); ok {
		t.Fatalf("ClockAt miss should be false")
	}
}

func TestHourlyLoad(t *testing.T) {
	clocks := map[string]Clock{
		"manha": {ID: "manha", Name: "M", Slots: []Slot{
			{Kind: SlotMusic, Category: library.CategoryMusic, Count: 5},
		}},
	}
	g := Grid{Cells: []Cell{
		{Weekday: time.Monday, Hour: 6, ClockID: "manha"},  // 5
		{Weekday: time.Monday, Hour: 7, ClockID: "ghost"},  // unknown clock → 0
		{Weekday: time.Tuesday, Hour: 6, ClockID: "manha"}, // other day, ignored
	}}
	load := HourlyLoad(g, clocks, time.Monday)
	if load[6] != 5 {
		t.Fatalf("hour 6 load = %d, want 5", load[6])
	}
	if load[7] != 0 {
		t.Fatalf("hour 7 (unknown clock) = %d, want 0", load[7])
	}
	if load[8] != 0 {
		t.Fatalf("hour 8 (empty) = %d, want 0", load[8])
	}
}

func TestDefaultGridValidates(t *testing.T) {
	g := DefaultGrid()
	if len(g.Cells) != 7*24 {
		t.Fatalf("default grid cells = %d, want 168", len(g.Cells))
	}
	if err := g.Validate(knownFromDefault()); err != nil {
		t.Fatalf("DefaultGrid invalid: %v", err)
	}
	// spot-check the daypart mapping.
	if id, _ := g.ClockAt(time.Wednesday, 9); id != "manha" {
		t.Fatalf("wed 09h = %q, want manha", id)
	}
	if id, _ := g.ClockAt(time.Wednesday, 15); id != "tarde" {
		t.Fatalf("wed 15h = %q, want tarde", id)
	}
	if id, _ := g.ClockAt(time.Wednesday, 3); id != "madrugada" {
		t.Fatalf("wed 03h = %q, want madrugada", id)
	}
}
