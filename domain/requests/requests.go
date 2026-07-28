// Package requests is the listener-request bounded context: song requests and
// dedications submitted by the audience and moderated by the operator. Unlike
// the report contexts (which are stateless projections over the immutable play
// log), a Request has its own lifecycle — it is born pending and transitions
// through a fixed matrix (pending→approved/rejected, approved→played/rejected).
// The domain is pure: no clock, no persistence, no external deps. Callers pass
// the timestamp in; transitions are immutable copy-return.
package requests

import (
	"errors"
	"strings"
	"time"
)

// Status is the moderation state of a listener request.
type Status string

const (
	// StatusPending is a freshly submitted, not-yet-moderated request.
	StatusPending Status = "pending"
	// StatusApproved means the operator accepted it (may still be played).
	StatusApproved Status = "approved"
	// StatusRejected means the operator declined it (terminal).
	StatusRejected Status = "rejected"
	// StatusPlayed means an approved request actually aired (terminal).
	StatusPlayed Status = "played"
)

// Valid reports whether s is one of the known statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusRejected, StatusPlayed:
		return true
	default:
		return false
	}
}

// Domain errors.
var (
	// ErrEmptyListener is returned when the listener name is blank.
	ErrEmptyListener = errors.New("requests: listener name is required")
	// ErrEmptyRequest is returned when the song hint is blank.
	ErrEmptyRequest = errors.New("requests: song hint is required")
	// ErrEmptyCreatedAt is returned when createdAt is the zero time.
	ErrEmptyCreatedAt = errors.New("requests: createdAt is required")
	// ErrInvalidTransition is returned when a transition is not allowed by the
	// state matrix (e.g. pending→played, played→anything, rejected→anything).
	ErrInvalidTransition = errors.New("requests: invalid status transition")
)

// allowedTransitions is the moderation matrix: from-status → set of reachable
// statuses. Anything not listed is rejected with ErrInvalidTransition. This is
// the single source of truth for the lifecycle invariant.
var allowedTransitions = map[Status]map[Status]bool{
	StatusPending:  {StatusApproved: true, StatusRejected: true},
	StatusApproved: {StatusPlayed: true, StatusRejected: true},
}

// Request is a listener's song request / dedication and its moderation state.
// It is immutable: transitions return a new copy.
type Request struct {
	ID           string
	ListenerName string
	SongHint     string
	Dedication   string
	Status       Status
	CreatedAt    time.Time
	HandledAt    time.Time // zero until the first transition
}

// NewRequest builds a pending request. listener and songHint are trimmed and
// required; dedication is optional; createdAt must be non-zero. The request is
// born pending with a zero HandledAt.
func NewRequest(id, listener, songHint, dedication string, createdAt time.Time) (Request, error) {
	listener = strings.TrimSpace(listener)
	songHint = strings.TrimSpace(songHint)
	dedication = strings.TrimSpace(dedication)
	if listener == "" {
		return Request{}, ErrEmptyListener
	}
	if songHint == "" {
		return Request{}, ErrEmptyRequest
	}
	if createdAt.IsZero() {
		return Request{}, ErrEmptyCreatedAt
	}
	return Request{
		ID:           strings.TrimSpace(id),
		ListenerName: listener,
		SongHint:     songHint,
		Dedication:   dedication,
		Status:       StatusPending,
		CreatedAt:    createdAt,
	}, nil
}

// transition returns a copy of r moved to next, stamping HandledAt=at, or
// ErrInvalidTransition if the matrix forbids it.
func (r Request) transition(next Status, at time.Time) (Request, error) {
	if !allowedTransitions[r.Status][next] {
		return Request{}, ErrInvalidTransition
	}
	r.Status = next
	r.HandledAt = at
	return r, nil
}

// Approve moves a pending request to approved (copy-return).
func (r Request) Approve(at time.Time) (Request, error) {
	return r.transition(StatusApproved, at)
}

// Reject moves a pending or approved request to rejected (copy-return).
func (r Request) Reject(at time.Time) (Request, error) {
	return r.transition(StatusRejected, at)
}

// MarkPlayed moves an approved request to played (copy-return).
func (r Request) MarkPlayed(at time.Time) (Request, error) {
	return r.transition(StatusPlayed, at)
}
