package memory

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

func sampleClock(id string) schedule.Clock {
	return schedule.Clock{
		ID:   id,
		Name: id,
		Slots: []schedule.Slot{
			{Kind: schedule.SlotMusic, Category: library.CategoryMusic, Count: 2},
		},
	}
}

func TestClockRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewClockRepo()

	// empty
	if list, err := r.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List empty = %v, %v", list, err)
	}
	if _, err := r.Get(ctx, "nope"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("Get missing = %v, want ErrClockNotFound", err)
	}
	if err := r.Delete(ctx, "nope"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("Delete missing = %v, want ErrClockNotFound", err)
	}

	// save two, list ordered
	if err := r.Save(ctx, sampleClock("b")); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(ctx, sampleClock("a")); err != nil {
		t.Fatal(err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("List = %v, %v", list, err)
	}

	// get + upsert
	got, err := r.Get(ctx, "a")
	if err != nil || got.Name != "a" {
		t.Fatalf("Get a = %v, %v", got, err)
	}
	upd := sampleClock("a")
	upd.Name = "AA"
	if err := r.Save(ctx, upd); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get(ctx, "a"); got.Name != "AA" {
		t.Fatalf("upsert Name = %q, want AA", got.Name)
	}

	// delete
	if err := r.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "a"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}
