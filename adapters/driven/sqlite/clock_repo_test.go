package sqlite

import (
	"context"
	"errors"
	"testing"

	"autodj/domain/library"
	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

func clockFixture(id string) schedule.Clock {
	return schedule.Clock{
		ID:   id,
		Name: id,
		Slots: []schedule.Slot{
			{Kind: schedule.SlotMusic, Category: library.CategoryMusic, Count: 3},
			{Kind: schedule.SlotJingle, Category: library.CategoryJingle, Count: 1},
		},
	}
}

func TestClockRepoSQLite(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	cr := repo.Clocks()
	ctx := context.Background()

	// empty
	if list, err := cr.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List empty = %v, %v", list, err)
	}
	if _, err := cr.Get(ctx, "nope"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if err := cr.Delete(ctx, "nope"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("Delete missing = %v", err)
	}

	// save two, list ordered
	if err := cr.Save(ctx, clockFixture("b")); err != nil {
		t.Fatal(err)
	}
	if err := cr.Save(ctx, clockFixture("a")); err != nil {
		t.Fatal(err)
	}
	list, err := cr.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("List = %v, %v", list, err)
	}
	if len(list[0].Slots) != 2 {
		t.Fatalf("slots round-trip = %d, want 2", len(list[0].Slots))
	}

	// get + upsert
	got, err := cr.Get(ctx, "a")
	if err != nil || got.Name != "a" {
		t.Fatalf("Get a = %v, %v", got, err)
	}
	upd := clockFixture("a")
	upd.Name = "AA"
	if err := cr.Save(ctx, upd); err != nil {
		t.Fatal(err)
	}
	if got, _ := cr.Get(ctx, "a"); got.Name != "AA" {
		t.Fatalf("upsert Name = %q, want AA", got.Name)
	}

	// delete
	if err := cr.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := cr.Get(ctx, "a"); !errors.Is(err, outbound.ErrClockNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}

func TestClockRepoSQLiteDecodeError(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if _, err := repo.db.Exec(
		`INSERT INTO clocks (id, name, data) VALUES ('x','X',?)`, "{not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cr := repo.Clocks()
	if _, err := cr.Get(context.Background(), "x"); err == nil {
		t.Fatal("expected decode error on Get")
	}
	if _, err := cr.List(context.Background()); err == nil {
		t.Fatal("expected decode error on List")
	}
}

func TestClockRepoSQLiteClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	cr := repo.Clocks()
	closeFn()
	ctx := context.Background()
	if _, err := cr.List(ctx); err == nil {
		t.Fatal("expected List error on closed db")
	}
	if _, err := cr.Get(ctx, "x"); err == nil {
		t.Fatal("expected Get error on closed db")
	}
	if err := cr.Save(ctx, clockFixture("x")); err == nil {
		t.Fatal("expected Save error on closed db")
	}
	if err := cr.Delete(ctx, "x"); err == nil {
		t.Fatal("expected Delete error on closed db")
	}
}
