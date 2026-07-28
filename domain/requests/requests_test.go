package requests

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
var t1 = time.Date(2026, 7, 24, 11, 0, 0, 0, time.UTC)

func TestNewRequest_OK(t *testing.T) {
	r, err := NewRequest("  req-1 ", "  Ana ", "  aquela dos anos 90 ", "  pro João ", t0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.ID != "req-1" || r.ListenerName != "Ana" || r.SongHint != "aquela dos anos 90" || r.Dedication != "pro João" {
		t.Fatalf("bad trim: %+v", r)
	}
	if r.Status != StatusPending {
		t.Fatalf("want pending, got %s", r.Status)
	}
	if !r.HandledAt.IsZero() {
		t.Fatalf("HandledAt should be zero on new request")
	}
	if !r.CreatedAt.Equal(t0) {
		t.Fatalf("CreatedAt mismatch")
	}
}

func TestNewRequest_OptionalDedication(t *testing.T) {
	r, err := NewRequest("id", "Ana", "song", "", t0)
	if err != nil {
		t.Fatalf("dedication should be optional: %v", err)
	}
	if r.Dedication != "" {
		t.Fatalf("want empty dedication")
	}
}

func TestNewRequest_Validation(t *testing.T) {
	cases := []struct {
		name              string
		listener, song    string
		created           time.Time
		want              error
	}{
		{"empty listener", "   ", "song", t0, ErrEmptyListener},
		{"empty song", "Ana", "  ", t0, ErrEmptyRequest},
		{"zero created", "Ana", "song", time.Time{}, ErrEmptyCreatedAt},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewRequest("id", c.listener, c.song, "", c.created)
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestStatus_Valid(t *testing.T) {
	for _, s := range []Status{StatusPending, StatusApproved, StatusRejected, StatusPlayed} {
		if !s.Valid() {
			t.Fatalf("%s should be valid", s)
		}
	}
	if Status("bogus").Valid() {
		t.Fatalf("bogus should be invalid")
	}
}

func newPending(t *testing.T) Request {
	t.Helper()
	r, err := NewRequest("req-1", "Ana", "song", "", t0)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return r
}

func TestApprove_FromPending(t *testing.T) {
	r := newPending(t)
	got, err := r.Approve(t1)
	if err != nil {
		t.Fatalf("approve pending: %v", err)
	}
	if got.Status != StatusApproved || !got.HandledAt.Equal(t1) {
		t.Fatalf("bad approve: %+v", got)
	}
	// immutability: original untouched.
	if r.Status != StatusPending || !r.HandledAt.IsZero() {
		t.Fatalf("original mutated: %+v", r)
	}
}

func TestReject_FromPendingAndApproved(t *testing.T) {
	p := newPending(t)
	rej, err := p.Reject(t1)
	if err != nil || rej.Status != StatusRejected || !rej.HandledAt.Equal(t1) {
		t.Fatalf("reject pending: %+v %v", rej, err)
	}
	ap, _ := p.Approve(t1)
	rej2, err := ap.Reject(t1)
	if err != nil || rej2.Status != StatusRejected {
		t.Fatalf("reject approved: %+v %v", rej2, err)
	}
}

func TestMarkPlayed_OnlyFromApproved(t *testing.T) {
	p := newPending(t)
	if _, err := p.MarkPlayed(t1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pending→played must fail, got %v", err)
	}
	ap, _ := p.Approve(t1)
	played, err := ap.MarkPlayed(t1)
	if err != nil || played.Status != StatusPlayed || !played.HandledAt.Equal(t1) {
		t.Fatalf("approved→played: %+v %v", played, err)
	}
}

func TestInvalidTransitions(t *testing.T) {
	p := newPending(t)
	ap, _ := p.Approve(t1)
	rej, _ := p.Reject(t1)
	played, _ := ap.MarkPlayed(t1)

	// terminal states reject everything.
	if _, err := rej.Approve(t1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("rejected→approved must fail")
	}
	if _, err := rej.MarkPlayed(t1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("rejected→played must fail")
	}
	if _, err := played.Reject(t1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("played→rejected must fail")
	}
	if _, err := played.Approve(t1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("played→approved must fail")
	}
	// approved cannot be re-approved.
	if _, err := ap.Approve(t1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("approved→approved must fail")
	}
}
