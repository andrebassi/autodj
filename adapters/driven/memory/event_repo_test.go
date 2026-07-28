package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

func sampleEvent(id string) schedule.ScheduledEvent {
	return schedule.ScheduledEvent{
		ID:       id,
		Kind:     schedule.EventHoraCerta,
		Days:     []time.Weekday{time.Monday},
		Hour:     12,
		Minute:   0,
		Priority: 80,
	}
}

func TestEventRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewEventRepo()

	// empty
	if list, err := r.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List empty = %v, %v", list, err)
	}
	if _, err := r.Get(ctx, "nope"); !errors.Is(err, outbound.ErrEventNotFound) {
		t.Fatalf("Get missing = %v, want ErrEventNotFound", err)
	}
	if err := r.Delete(ctx, "nope"); !errors.Is(err, outbound.ErrEventNotFound) {
		t.Fatalf("Delete missing = %v, want ErrEventNotFound", err)
	}

	// save two, list ordered by id
	if err := r.Save(ctx, sampleEvent("b")); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(ctx, sampleEvent("a")); err != nil {
		t.Fatal(err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("List = %v, %v", list, err)
	}

	// get + upsert
	got, err := r.Get(ctx, "a")
	if err != nil || got.Hour != 12 {
		t.Fatalf("Get a = %v, %v", got, err)
	}
	upd := sampleEvent("a")
	upd.Hour = 9
	if err := r.Save(ctx, upd); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get(ctx, "a"); got.Hour != 9 {
		t.Fatalf("upsert Hour = %d, want 9", got.Hour)
	}

	// delete
	if err := r.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "a"); !errors.Is(err, outbound.ErrEventNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}
