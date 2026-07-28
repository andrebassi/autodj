package staffschedule

import (
	"errors"
	"testing"
	"time"
)

func TestNewShiftValidation(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		announcerID   string
		announcerName string
		weekday       time.Weekday
		startMin      int
		endMin        int
		want          error
	}{
		{"ok", "s1", "a1", "Marina Vale", time.Monday, 8 * 60, 12 * 60, nil},
		{"trim-ok", "  s1  ", "", "  Marina Vale  ", time.Sunday, 0, 60, nil},
		{"empty-id", "  ", "a1", "Marina", time.Monday, 0, 60, ErrEmptyID},
		{"empty-announcer", "s1", "a1", "   ", time.Monday, 0, 60, ErrEmptyAnnouncer},
		{"weekday-low", "s1", "a1", "Marina", time.Weekday(-1), 0, 60, ErrBadWeekday},
		{"weekday-high", "s1", "a1", "Marina", time.Weekday(7), 0, 60, ErrBadWeekday},
		{"start-neg", "s1", "a1", "Marina", time.Monday, -1, 60, ErrBadRange},
		{"end-over", "s1", "a1", "Marina", time.Monday, 0, 1441, ErrBadRange},
		{"start-ge-end", "s1", "a1", "Marina", time.Monday, 600, 600, ErrBadRange},
		{"full-day", "s1", "a1", "Marina", time.Saturday, 0, 1440, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewShift(tt.id, tt.announcerID, tt.announcerName, tt.weekday, tt.startMin, tt.endMin)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want == nil {
				if s.AnnouncerName == "" {
					t.Fatalf("expected trimmed announcer name, got empty")
				}
				if s.ID == "" {
					t.Fatalf("expected trimmed id, got empty")
				}
			}
		})
	}
}

func TestShiftCovers(t *testing.T) {
	s := mustShift(t, "s1", "Marina", time.Monday, 8*60, 12*60)
	cases := []struct {
		name    string
		weekday time.Weekday
		minute  int
		want    bool
	}{
		{"inside", time.Monday, 10 * 60, true},
		{"at-start", time.Monday, 8 * 60, true},
		{"at-end-exclusive", time.Monday, 12 * 60, false},
		{"before", time.Monday, 8*60 - 1, false},
		{"other-weekday", time.Tuesday, 10 * 60, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.Covers(c.weekday, c.minute); got != c.want {
				t.Fatalf("Covers(%v,%d) = %v, want %v", c.weekday, c.minute, got, c.want)
			}
		})
	}
}

func TestBuildRosterOverlap(t *testing.T) {
	a := mustShift(t, "a", "Marina", time.Monday, 8*60, 10*60)
	b := mustShift(t, "b", "Caio", time.Monday, 9*60, 11*60)   // overlaps a
	adj := mustShift(t, "c", "Caio", time.Monday, 10*60, 12*60) // adjacent to a, no overlap
	other := mustShift(t, "d", "Bea", time.Tuesday, 9*60, 11*60)

	if _, err := BuildRoster([]Shift{a, b}); !errors.Is(err, ErrOverlap) {
		t.Fatalf("expected ErrOverlap, got %v", err)
	}
	// same weekday + same start exercises the id tie-break in the sort comparator
	sameStart := mustShift(t, "b2", "Duda", time.Monday, 8*60, 9*60)
	if _, err := BuildRoster([]Shift{a, sameStart}); !errors.Is(err, ErrOverlap) {
		t.Fatalf("expected ErrOverlap for same-start shifts, got %v", err)
	}
	if _, err := BuildRoster([]Shift{a, adj}); err != nil {
		t.Fatalf("adjacent shifts should not overlap: %v", err)
	}
	if _, err := BuildRoster([]Shift{b, other}); err != nil {
		t.Fatalf("different weekdays should not overlap: %v", err)
	}
}

func TestBuildRosterValidatesEach(t *testing.T) {
	bad := Shift{ID: "x", AnnouncerName: "", Weekday: time.Monday, StartMin: 0, EndMin: 60}
	if _, err := BuildRoster([]Shift{bad}); !errors.Is(err, ErrEmptyAnnouncer) {
		t.Fatalf("expected ErrEmptyAnnouncer, got %v", err)
	}
}

func TestRosterShiftsOrdered(t *testing.T) {
	late := mustShift(t, "z", "Marina", time.Tuesday, 8*60, 9*60)
	early := mustShift(t, "a", "Caio", time.Monday, 14*60, 15*60)
	earlier := mustShift(t, "m", "Bea", time.Monday, 6*60, 7*60)

	r, err := BuildRoster([]Shift{late, early, earlier})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got := r.Shifts()
	wantOrder := []string{"m", "a", "z"} // Mon06, Mon14, Tue08
	if len(got) != len(wantOrder) {
		t.Fatalf("len = %d, want %d", len(got), len(wantOrder))
	}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Fatalf("order[%d] = %s, want %s", i, got[i].ID, id)
		}
	}
	// mutating the returned slice must not affect the roster
	got[0].ID = "mutated"
	if r.Shifts()[0].ID != "m" {
		t.Fatalf("Shifts() should return a copy")
	}
}

func TestRosterForWeekdayAndOnAir(t *testing.T) {
	mon1 := mustShift(t, "a", "Marina", time.Monday, 6*60, 10*60)
	mon2 := mustShift(t, "b", "Caio", time.Monday, 10*60, 14*60)
	tue := mustShift(t, "c", "Bea", time.Tuesday, 8*60, 12*60)

	r, err := BuildRoster([]Shift{mon2, mon1, tue})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	monShifts := r.ForWeekday(time.Monday)
	if len(monShifts) != 2 || monShifts[0].ID != "a" || monShifts[1].ID != "b" {
		t.Fatalf("ForWeekday(Mon) = %+v", monShifts)
	}
	if got := r.ForWeekday(time.Sunday); got != nil {
		t.Fatalf("ForWeekday(Sun) = %+v, want nil", got)
	}

	if s, ok := r.OnAir(time.Monday, 8*60); !ok || s.ID != "a" {
		t.Fatalf("OnAir(Mon 08:00) = %+v, %v", s, ok)
	}
	if s, ok := r.OnAir(time.Monday, 12*60); !ok || s.ID != "b" {
		t.Fatalf("OnAir(Mon 12:00) = %+v, %v", s, ok)
	}
	if _, ok := r.OnAir(time.Monday, 5*60); ok {
		t.Fatalf("OnAir(Mon 05:00) should be off air")
	}
	if _, ok := r.OnAir(time.Wednesday, 9*60); ok {
		t.Fatalf("OnAir(Wed) should be off air")
	}
}

func TestMinuteOfDay(t *testing.T) {
	at := time.Date(2026, 7, 24, 9, 30, 45, 0, time.UTC)
	if got := MinuteOfDay(at); got != 9*60+30 {
		t.Fatalf("MinuteOfDay = %d, want %d", got, 9*60+30)
	}
}

func mustShift(t *testing.T, id, name string, weekday time.Weekday, start, end int) Shift {
	t.Helper()
	s, err := NewShift(id, "", name, weekday, start, end)
	if err != nil {
		t.Fatalf("mustShift(%s): %v", id, err)
	}
	return s
}
