package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/requests"
	"autodj/ports/outbound"
)

func newRequestRepo(t *testing.T) *RequestRepo {
	t.Helper()
	tr, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = closeFn() })
	return tr.Requests()
}

func mkReq(t *testing.T, id, listener, song string, at time.Time) requests.Request {
	t.Helper()
	r, err := requests.NewRequest(id, listener, song, "dedic-"+id, at)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return r
}

func TestRequestRepoSQLite(t *testing.T) {
	ctx := context.Background()
	r := newRequestRepo(t)
	base := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)

	// unknown id → ErrRequestNotFound
	if _, err := r.Get(ctx, "nope"); !errors.Is(err, outbound.ErrRequestNotFound) {
		t.Fatalf("want ErrRequestNotFound, got %v", err)
	}

	// empty list is fine
	if all, err := r.List(ctx); err != nil || len(all) != 0 {
		t.Fatalf("empty list: %d %v", len(all), err)
	}

	b := mkReq(t, "b", "Bia", "song B", base.Add(time.Hour))
	a := mkReq(t, "a", "Ana", "song A", base)
	for _, req := range []requests.Request{b, a} {
		if err := r.Save(ctx, req); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	// Get round-trips the full request
	got, err := r.Get(ctx, "a")
	if err != nil {
		t.Fatalf("get a: %v", err)
	}
	if got.ListenerName != "Ana" || got.SongHint != "song A" || got.Status != requests.StatusPending ||
		got.Dedication != "dedic-a" || !got.CreatedAt.Equal(base) {
		t.Fatalf("round-trip: %+v", got)
	}

	// List ordered by id
	all, err := r.List(ctx)
	if err != nil || len(all) != 2 || all[0].ID != "a" || all[1].ID != "b" {
		t.Fatalf("list order: %+v %v", all, err)
	}

	// upsert: approve a and re-save updates status + handledAt
	appr, err := a.Approve(base.Add(2 * time.Hour))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := r.Save(ctx, appr); err != nil {
		t.Fatalf("save approved: %v", err)
	}
	got2, err := r.Get(ctx, "a")
	if err != nil || got2.Status != requests.StatusApproved || got2.HandledAt.IsZero() {
		t.Fatalf("upsert approved: %+v %v", got2, err)
	}
	if n, err := r.List(ctx); err != nil || len(n) != 2 {
		t.Fatalf("upsert must not grow rows: %d %v", len(n), err)
	}
}

func TestRequestRepoSQLiteDecodeError(t *testing.T) {
	ctx := context.Background()
	r := newRequestRepo(t)
	// corrupt row: data is not valid JSON → decode error on Get and List.
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO requests (id, status, created_at, data) VALUES (?, ?, ?, ?)`,
		"bad", "pending", "2026-07-24T10:00:00Z", "{not json"); err != nil {
		t.Fatalf("seed corrupt: %v", err)
	}
	if _, err := r.Get(ctx, "bad"); err == nil {
		t.Fatalf("want decode error on Get")
	}
	if _, err := r.List(ctx); err == nil {
		t.Fatalf("want decode error on List")
	}
}

func TestRequestRepoSQLiteClosedDB(t *testing.T) {
	ctx := context.Background()
	tr, closeFn, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	r := tr.Requests()
	_ = closeFn()
	req := mkReq(t, "a", "Ana", "song", time.Now())
	if err := r.Save(ctx, req); err == nil {
		t.Fatalf("want Save error on closed db")
	}
	if _, err := r.List(ctx); err == nil {
		t.Fatalf("want List error on closed db")
	}
}
