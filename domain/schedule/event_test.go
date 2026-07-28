package schedule

import (
	"errors"
	"testing"
	"time"
)

// ev builds a minimal valid event, overridable by the caller after.
func ev(id string, kind EventKind, day time.Weekday, h, m, prio int) ScheduledEvent {
	return ScheduledEvent{
		ID:       id,
		Kind:     kind,
		Days:     []time.Weekday{day},
		Hour:     h,
		Minute:   m,
		Priority: prio,
	}
}

func TestDefaultEventsValidate(t *testing.T) {
	events := DefaultEvents()
	if len(events) == 0 {
		t.Fatal("DefaultEvents empty")
	}
	for _, e := range events {
		if err := e.Validate(); err != nil {
			t.Fatalf("default event %q invalid: %v", e.ID, err)
		}
	}
}

func TestScheduledEventValidate(t *testing.T) {
	base := ev("e1", EventComercial, time.Monday, 8, 30, 50)
	cases := []struct {
		name string
		mut  func(*ScheduledEvent)
		want error
	}{
		{"ok", func(*ScheduledEvent) {}, nil},
		{"empty id", func(e *ScheduledEvent) { e.ID = "" }, ErrEventEmptyID},
		{"bad kind", func(e *ScheduledEvent) { e.Kind = "nope" }, ErrBadEventKind},
		{"hour low", func(e *ScheduledEvent) { e.Hour = -1 }, ErrEventHour},
		{"hour high", func(e *ScheduledEvent) { e.Hour = 24 }, ErrEventHour},
		{"minute low", func(e *ScheduledEvent) { e.Minute = -1 }, ErrEventMinute},
		{"minute high", func(e *ScheduledEvent) { e.Minute = 60 }, ErrEventMinute},
		{"no days", func(e *ScheduledEvent) { e.Days = nil }, ErrEventNoDays},
		{"bad day", func(e *ScheduledEvent) { e.Days = []time.Weekday{9} }, ErrEventBadDay},
		{"dup day", func(e *ScheduledEvent) {
			e.Days = []time.Weekday{time.Monday, time.Monday}
		}, ErrEventBadDay},
		{"prio low", func(e *ScheduledEvent) { e.Priority = -1 }, ErrEventPriority},
		{"prio high", func(e *ScheduledEvent) { e.Priority = 101 }, ErrEventPriority},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			e.Days = append([]time.Weekday(nil), base.Days...)
			tc.mut(&e)
			if err := e.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestScheduledEventDueAt(t *testing.T) {
	// Monday 2024-01-01 08:30:00.
	e := ev("e1", EventComercial, time.Monday, 8, 30, 50)
	mon0830 := time.Date(2024, 1, 1, 8, 30, 45, 0, time.UTC) // seconds ignored
	if !e.DueAt(mon0830) {
		t.Fatal("expected due at Monday 08:30")
	}
	// wrong minute.
	if e.DueAt(time.Date(2024, 1, 1, 8, 31, 0, 0, time.UTC)) {
		t.Fatal("must not be due at 08:31")
	}
	// wrong hour.
	if e.DueAt(time.Date(2024, 1, 1, 9, 30, 0, 0, time.UTC)) {
		t.Fatal("must not be due at 09:30")
	}
	// right time, wrong weekday (Tuesday 2024-01-02).
	if e.DueAt(time.Date(2024, 1, 2, 8, 30, 0, 0, time.UTC)) {
		t.Fatal("must not be due on Tuesday")
	}
}

func TestDueEventsOrdersByPriorityDesc(t *testing.T) {
	at := time.Date(2024, 1, 1, 8, 30, 0, 0, time.UTC) // Monday 08:30
	events := []ScheduledEvent{
		ev("low", EventComercial, time.Monday, 8, 30, 10),
		ev("high", EventVozDoBrasil, time.Monday, 8, 30, 90),
		ev("other-time", EventHoraCerta, time.Monday, 9, 0, 100), // not due
		ev("mid", EventProgramete, time.Monday, 8, 30, 50),
	}
	due := DueEvents(events, at)
	if len(due) != 3 {
		t.Fatalf("len = %d, want 3", len(due))
	}
	if due[0].ID != "high" || due[1].ID != "mid" || due[2].ID != "low" {
		t.Fatalf("order = %s,%s,%s", due[0].ID, due[1].ID, due[2].ID)
	}
}

func TestDueEventsTieStable(t *testing.T) {
	at := time.Date(2024, 1, 1, 8, 30, 0, 0, time.UTC)
	events := []ScheduledEvent{
		ev("a", EventComercial, time.Monday, 8, 30, 50),
		ev("b", EventComercial, time.Monday, 8, 30, 50),
		ev("c", EventComercial, time.Monday, 8, 30, 50),
	}
	due := DueEvents(events, at)
	if len(due) != 3 || due[0].ID != "a" || due[1].ID != "b" || due[2].ID != "c" {
		t.Fatalf("tie not stable: %+v", due)
	}
}

func TestDueEventsNoneDue(t *testing.T) {
	at := time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC)
	events := []ScheduledEvent{ev("a", EventComercial, time.Monday, 8, 30, 50)}
	if due := DueEvents(events, at); len(due) != 0 {
		t.Fatalf("expected none due, got %d", len(due))
	}
}
