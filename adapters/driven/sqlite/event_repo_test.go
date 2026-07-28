package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

func eventFixture(id string) schedule.ScheduledEvent {
	return schedule.ScheduledEvent{
		ID:       id,
		Kind:     schedule.EventComercial,
		Days:     []time.Weekday{time.Monday, time.Wednesday},
		Hour:     8,
		Minute:   30,
		Priority: 50,
		MediaRef: "commercial",
	}
}

func TestEventRepoSQLite(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	er := repo.Events()
	ctx := context.Background()

	// empty
	if list, err := er.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List empty = %v, %v", list, err)
	}
	if _, err := er.Get(ctx, "nope"); !errors.Is(err, outbound.ErrEventNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if err := er.Delete(ctx, "nope"); !errors.Is(err, outbound.ErrEventNotFound) {
		t.Fatalf("Delete missing = %v", err)
	}

	// save two, list ordered
	if err := er.Save(ctx, eventFixture("b")); err != nil {
		t.Fatal(err)
	}
	if err := er.Save(ctx, eventFixture("a")); err != nil {
		t.Fatal(err)
	}
	list, err := er.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("List = %v, %v", list, err)
	}
	if len(list[0].Days) != 2 || list[0].MediaRef != "commercial" {
		t.Fatalf("round-trip = %+v", list[0])
	}

	// get + upsert
	got, err := er.Get(ctx, "a")
	if err != nil || got.Hour != 8 {
		t.Fatalf("Get a = %v, %v", got, err)
	}
	upd := eventFixture("a")
	upd.Hour = 20
	if err := er.Save(ctx, upd); err != nil {
		t.Fatal(err)
	}
	if got, _ := er.Get(ctx, "a"); got.Hour != 20 {
		t.Fatalf("upsert Hour = %d, want 20", got.Hour)
	}

	// delete
	if err := er.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := er.Get(ctx, "a"); !errors.Is(err, outbound.ErrEventNotFound) {
		t.Fatalf("Get after delete = %v", err)
	}
}

func TestEventRepoSQLiteDecodeError(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer closeFn()
	if _, err := repo.db.Exec(
		`INSERT INTO events (id, kind, data) VALUES ('x','hora_certa',?)`, "{not json"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	er := repo.Events()
	if _, err := er.Get(context.Background(), "x"); err == nil {
		t.Fatal("expected decode error on Get")
	}
	if _, err := er.List(context.Background()); err == nil {
		t.Fatal("expected decode error on List")
	}
}

func TestEventRepoSQLiteClosedDB(t *testing.T) {
	repo, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	er := repo.Events()
	closeFn()
	ctx := context.Background()
	if _, err := er.List(ctx); err == nil {
		t.Fatal("expected List error on closed db")
	}
	if _, err := er.Get(ctx, "x"); err == nil {
		t.Fatal("expected Get error on closed db")
	}
	if err := er.Save(ctx, eventFixture("x")); err == nil {
		t.Fatal("expected Save error on closed db")
	}
	if err := er.Delete(ctx, "x"); err == nil {
		t.Fatal("expected Delete error on closed db")
	}
}
