package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/staffschedule"
	"autodj/ports/outbound"
)

func sampleShift(t *testing.T, id string, weekday time.Weekday, start, end int) staffschedule.Shift {
	t.Helper()
	s, err := staffschedule.NewShift(id, "aid-"+id, "Loc "+id, weekday, start, end)
	if err != nil {
		t.Fatalf("new shift: %v", err)
	}
	return s
}

func TestStaffRepoMemory(t *testing.T) {
	ctx := context.Background()
	r := NewStaffRepo()

	// empty
	if list, err := r.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List empty = %v, %v", list, err)
	}
	if _, err := r.Get(ctx, "nope"); !errors.Is(err, outbound.ErrShiftNotFound) {
		t.Fatalf("Get missing = %v, want ErrShiftNotFound", err)
	}
	if err := r.Delete(ctx, "nope"); !errors.Is(err, outbound.ErrShiftNotFound) {
		t.Fatalf("Delete missing = %v, want ErrShiftNotFound", err)
	}

	// save three; List ordered by (weekday, start, id)
	if err := r.Save(ctx, sampleShift(t, "c", time.Monday, 10*60, 12*60)); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(ctx, sampleShift(t, "a", time.Monday, 8*60, 10*60)); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(ctx, sampleShift(t, "b", time.Sunday, 8*60, 9*60)); err != nil {
		t.Fatal(err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("List = %v, %v", list, err)
	}
	if list[0].ID != "b" || list[1].ID != "a" || list[2].ID != "c" {
		t.Fatalf("ordering = %s,%s,%s", list[0].ID, list[1].ID, list[2].ID)
	}

	// get + upsert (same id, moved to Wednesday)
	got, err := r.Get(ctx, "a")
	if err != nil || got.Weekday != time.Monday {
		t.Fatalf("Get a = %v, %v", got, err)
	}
	upd := sampleShift(t, "a", time.Wednesday, 14*60, 16*60)
	if err := r.Save(ctx, upd); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get(ctx, "a"); got.Weekday != time.Wednesday {
		t.Fatalf("upsert Weekday = %v, want Wednesday", got.Weekday)
	}
	if list, _ := r.List(ctx); len(list) != 3 {
		t.Fatalf("upsert changed count = %d, want 3", len(list))
	}

	// delete
	if err := r.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "a"); !errors.Is(err, outbound.ErrShiftNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}
