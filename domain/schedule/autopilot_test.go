package schedule

import (
	"testing"
	"time"
)

func TestDecide_OnlyDueEvent_InjectsNoRefill(t *testing.T) {
	now := time.Date(2026, 7, 24, 19, 0, 0, 0, time.UTC)
	due := []ScheduledEvent{
		{ID: "b", Kind: EventComercial, Priority: 10},
		{ID: "a", Kind: EventVozDoBrasil, Priority: 90},
	}
	// queue above low-water so no refill; grid present.
	d := Decide(now, 10, 5, due, true)
	if d.Refill {
		t.Fatalf("expected no refill when queueLen >= lowWater")
	}
	if len(d.Inject) != 2 {
		t.Fatalf("expected 2 injections, got %d", len(d.Inject))
	}
	if d.Inject[0].ID != "a" || d.Inject[1].ID != "b" {
		t.Fatalf("expected priority desc order [a,b], got [%s,%s]", d.Inject[0].ID, d.Inject[1].ID)
	}
}

func TestDecide_OnlyLowQueue_RefillsNoInject(t *testing.T) {
	now := time.Date(2026, 7, 24, 3, 0, 0, 0, time.UTC)
	d := Decide(now, 2, 5, nil, true)
	if !d.Refill {
		t.Fatalf("expected refill when queueLen < lowWater and grid present")
	}
	if len(d.Inject) != 0 {
		t.Fatalf("expected no injections, got %d", len(d.Inject))
	}
}

func TestDecide_Both_InjectAndRefill(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	due := []ScheduledEvent{{ID: "x", Kind: EventHoraCerta, Priority: 50}}
	d := Decide(now, 1, 5, due, true)
	if !d.Refill {
		t.Fatalf("expected refill")
	}
	if len(d.Inject) != 1 || d.Inject[0].ID != "x" {
		t.Fatalf("expected inject [x], got %v", d.Inject)
	}
}

func TestDecide_Neither(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	d := Decide(now, 10, 5, nil, true)
	if d.Refill {
		t.Fatalf("expected no refill")
	}
	if len(d.Inject) != 0 {
		t.Fatalf("expected no injections, got %d", len(d.Inject))
	}
}

func TestDecide_NoGrade_NeverRefills(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	// queue is low but there is no grid programming the hour → no refill.
	d := Decide(now, 0, 5, nil, false)
	if d.Refill {
		t.Fatalf("expected no refill without grid programming")
	}
}

func TestDecide_StableTieOrder(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	due := []ScheduledEvent{
		{ID: "first", Priority: 50},
		{ID: "second", Priority: 50},
		{ID: "third", Priority: 50},
	}
	d := Decide(now, 10, 5, due, false)
	if d.Inject[0].ID != "first" || d.Inject[1].ID != "second" || d.Inject[2].ID != "third" {
		t.Fatalf("expected stable input order on priority tie, got %v", d.Inject)
	}
}

func TestDecide_InjectIsCopy(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	due := []ScheduledEvent{{ID: "a", Priority: 1}, {ID: "b", Priority: 2}}
	d := Decide(now, 10, 5, due, false)
	// mutating the decision must not touch the caller's slice.
	d.Inject[0].ID = "mutated"
	if due[0].ID == "mutated" || due[1].ID == "mutated" {
		t.Fatalf("Decide must not alias the input slice")
	}
}
