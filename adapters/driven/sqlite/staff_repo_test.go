package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/staffschedule"
	"autodj/ports/outbound"
)

func newStaffRepo(t *testing.T) *StaffRepo {
	t.Helper()
	tr, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = closeFn() })
	return tr.Staff()
}

func mkShift(t *testing.T, id, name string, weekday time.Weekday, start, end int) staffschedule.Shift {
	t.Helper()
	s, err := staffschedule.NewShift(id, "aid-"+id, name, weekday, start, end)
	if err != nil {
		t.Fatalf("NewShift: %v", err)
	}
	return s
}

func TestStaffRepoSQLite(t *testing.T) {
	ctx := context.Background()
	r := newStaffRepo(t)

	// unknown id → ErrShiftNotFound
	if _, err := r.Get(ctx, "nope"); !errors.Is(err, outbound.ErrShiftNotFound) {
		t.Fatalf("want ErrShiftNotFound, got %v", err)
	}
	// delete unknown → ErrShiftNotFound
	if err := r.Delete(ctx, "nope"); !errors.Is(err, outbound.ErrShiftNotFound) {
		t.Fatalf("want ErrShiftNotFound on delete, got %v", err)
	}
	// empty list is fine
	if all, err := r.List(ctx); err != nil || len(all) != 0 {
		t.Fatalf("empty list: %d %v", len(all), err)
	}

	tue := mkShift(t, "b", "Bia", time.Tuesday, 8*60, 12*60)
	mon := mkShift(t, "a", "Ana", time.Monday, 8*60, 12*60)
	for _, s := range []staffschedule.Shift{tue, mon} {
		if err := r.Save(ctx, s); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	// Get round-trips the full shift
	got, err := r.Get(ctx, "a")
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if got.AnnouncerName != "Ana" || got.AnnouncerID != "aid-a" || got.Weekday != time.Monday ||
		got.StartMin != 8*60 || got.EndMin != 12*60 {
		t.Fatalf("round-trip: %+v", got)
	}

	// List ordered by (weekday, id): Monday before Tuesday
	all, err := r.List(ctx)
	if err != nil || len(all) != 2 || all[0].ID != "a" || all[1].ID != "b" {
		t.Fatalf("list order: %+v %v", all, err)
	}

	// upsert: move Ana to Wednesday, same id → no new row
	moved := mkShift(t, "a", "Ana", time.Wednesday, 14*60, 16*60)
	if err := r.Save(ctx, moved); err != nil {
		t.Fatalf("save moved: %v", err)
	}
	got2, err := r.Get(ctx, "a")
	if err != nil || got2.Weekday != time.Wednesday || got2.StartMin != 14*60 {
		t.Fatalf("upsert moved: %+v %v", got2, err)
	}
	if n, err := r.List(ctx); err != nil || len(n) != 2 {
		t.Fatalf("upsert must not grow rows: %d %v", len(n), err)
	}

	// Delete removes the row
	if err := r.Delete(ctx, "a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, "a"); !errors.Is(err, outbound.ErrShiftNotFound) {
		t.Fatalf("want ErrShiftNotFound after delete, got %v", err)
	}
}

func TestStaffRepoSQLiteDecodeError(t *testing.T) {
	ctx := context.Background()
	r := newStaffRepo(t)
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO staff_shifts (id, announcer_id, weekday, data) VALUES (?, ?, ?, ?)`,
		"bad", "aid", 1, "{not json"); err != nil {
		t.Fatalf("seed corrupt: %v", err)
	}
	if _, err := r.Get(ctx, "bad"); err == nil {
		t.Fatalf("want decode error on Get")
	}
	if _, err := r.List(ctx); err == nil {
		t.Fatalf("want decode error on List")
	}
}

func TestStaffRepoSQLiteClosedDB(t *testing.T) {
	ctx := context.Background()
	tr, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	r := tr.Staff()
	_ = closeFn()
	s := mkShift(t, "a", "Ana", time.Monday, 8*60, 12*60)
	if err := r.Save(ctx, s); err == nil {
		t.Fatalf("want Save error on closed db")
	}
	if _, err := r.List(ctx); err == nil {
		t.Fatalf("want List error on closed db")
	}
	if err := r.Delete(ctx, "a"); err == nil {
		t.Fatalf("want Delete error on closed db")
	}
}
