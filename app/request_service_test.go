package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/requests"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// fakeRequestRepo is an in-memory RequestRepo for the app tests, with an
// optional injected error to exercise propagation.
type fakeRequestRepo struct {
	items   map[string]requests.Request
	saveErr error
	listErr error
}

func newFakeRequestRepo() *fakeRequestRepo {
	return &fakeRequestRepo{items: map[string]requests.Request{}}
}

func (f *fakeRequestRepo) Save(_ context.Context, r requests.Request) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.items[r.ID] = r
	return nil
}

func (f *fakeRequestRepo) Get(_ context.Context, id string) (requests.Request, error) {
	r, ok := f.items[id]
	if !ok {
		return requests.Request{}, outbound.ErrRequestNotFound
	}
	return r, nil
}

func (f *fakeRequestRepo) List(context.Context) ([]requests.Request, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]requests.Request, 0, len(f.items))
	for _, r := range f.items {
		out = append(out, r)
	}
	return out, nil
}

var rqGen = time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

func TestRequestSubmit_GeneratesDistinctIDs(t *testing.T) {
	repo := newFakeRequestRepo()
	svc := NewRequestService(repo, fixedClock{rqGen})
	a, err := svc.Submit(context.Background(), "Ana", "song A", "")
	if err != nil {
		t.Fatalf("submit a: %v", err)
	}
	b, err := svc.Submit(context.Background(), "Bia", "song B", "dedic")
	if err != nil {
		t.Fatalf("submit b: %v", err)
	}
	if a.ID == b.ID {
		t.Fatalf("ids must differ: %s == %s", a.ID, b.ID)
	}
	if a.Status != requests.StatusPending || !a.CreatedAt.Equal(rqGen) {
		t.Fatalf("bad submit: %+v", a)
	}
	if len(repo.items) != 2 {
		t.Fatalf("want 2 persisted, got %d", len(repo.items))
	}
}

func TestRequestSubmit_ValidationPropagates(t *testing.T) {
	svc := NewRequestService(newFakeRequestRepo(), fixedClock{rqGen})
	if _, err := svc.Submit(context.Background(), "  ", "song", ""); !errors.Is(err, requests.ErrEmptyListener) {
		t.Fatalf("want ErrEmptyListener, got %v", err)
	}
	if _, err := svc.Submit(context.Background(), "Ana", "  ", ""); !errors.Is(err, requests.ErrEmptyRequest) {
		t.Fatalf("want ErrEmptyRequest, got %v", err)
	}
}

func TestRequestSubmit_SaveErrorPropagates(t *testing.T) {
	repo := newFakeRequestRepo()
	repo.saveErr = errors.New("boom")
	svc := NewRequestService(repo, fixedClock{rqGen})
	if _, err := svc.Submit(context.Background(), "Ana", "song", ""); err == nil {
		t.Fatalf("want save error")
	}
}

func TestRequestList_FilterByStatus(t *testing.T) {
	repo := newFakeRequestRepo()
	svc := NewRequestService(repo, fixedClock{rqGen})
	a, _ := svc.Submit(context.Background(), "Ana", "a", "")
	svc.Submit(context.Background(), "Bia", "b", "")
	if _, err := svc.Approve(context.Background(), a.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}

	all, err := svc.List(context.Background(), "")
	if err != nil || len(all) != 2 {
		t.Fatalf("all: %d %v", len(all), err)
	}
	appr, err := svc.List(context.Background(), "approved")
	if err != nil || len(appr) != 1 || appr[0].ID != a.ID {
		t.Fatalf("approved filter: %+v %v", appr, err)
	}
	pend, _ := svc.List(context.Background(), "pending")
	if len(pend) != 1 {
		t.Fatalf("pending filter: %d", len(pend))
	}
}

func TestRequestList_BadStatus(t *testing.T) {
	svc := NewRequestService(newFakeRequestRepo(), fixedClock{rqGen})
	if _, err := svc.List(context.Background(), "bogus"); !errors.Is(err, inbound.ErrBadStatus) {
		t.Fatalf("want ErrBadStatus, got %v", err)
	}
}

func TestRequestList_RepoErrorPropagates(t *testing.T) {
	repo := newFakeRequestRepo()
	repo.listErr = errors.New("db down")
	svc := NewRequestService(repo, fixedClock{rqGen})
	if _, err := svc.List(context.Background(), ""); err == nil {
		t.Fatalf("want list error")
	}
}

func TestRequestModeration_Transitions(t *testing.T) {
	repo := newFakeRequestRepo()
	svc := NewRequestService(repo, fixedClock{rqGen})
	r, _ := svc.Submit(context.Background(), "Ana", "song", "")

	ap, err := svc.Approve(context.Background(), r.ID)
	if err != nil || ap.Status != requests.StatusApproved || !ap.HandledAt.Equal(rqGen) {
		t.Fatalf("approve: %+v %v", ap, err)
	}
	played, err := svc.MarkPlayed(context.Background(), r.ID)
	if err != nil || played.Status != requests.StatusPlayed {
		t.Fatalf("markPlayed: %+v %v", played, err)
	}
}

func TestRequestModeration_NotFound(t *testing.T) {
	svc := NewRequestService(newFakeRequestRepo(), fixedClock{rqGen})
	for _, fn := range []func(context.Context, string) (requests.Request, error){svc.Approve, svc.Reject, svc.MarkPlayed} {
		if _, err := fn(context.Background(), "nope"); !errors.Is(err, outbound.ErrRequestNotFound) {
			t.Fatalf("want ErrRequestNotFound, got %v", err)
		}
	}
}

func TestRequestModeration_InvalidTransition(t *testing.T) {
	svc := NewRequestService(newFakeRequestRepo(), fixedClock{rqGen})
	r, _ := svc.Submit(context.Background(), "Ana", "song", "")
	// pending → played is forbidden.
	if _, err := svc.MarkPlayed(context.Background(), r.ID); !errors.Is(err, requests.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
	// reject then approve is forbidden.
	if _, err := svc.Reject(context.Background(), r.ID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := svc.Approve(context.Background(), r.ID); !errors.Is(err, requests.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition on rejected→approved, got %v", err)
	}
}

func TestRequestModeration_SaveErrorPropagates(t *testing.T) {
	repo := newFakeRequestRepo()
	svc := NewRequestService(repo, fixedClock{rqGen})
	r, _ := svc.Submit(context.Background(), "Ana", "song", "")
	repo.saveErr = errors.New("boom")
	if _, err := svc.Approve(context.Background(), r.ID); err == nil {
		t.Fatalf("want save error on moderate")
	}
}
