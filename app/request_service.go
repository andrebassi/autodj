package app

import (
	"context"
	"fmt"
	"sync"

	"autodj/domain/requests"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// RequestService owns the listener-request moderation queue (Story 8.3). Unlike
// the report services (stateless projections), it has real state persisted via
// RequestRepo. It composes the repo and a Clock: Submit stamps CreatedAt and the
// transitions stamp HandledAt from the injected clock, so ids and timestamps are
// deterministic under a fake clock. Ids are generated from the clock plus a
// monotonic per-service counter under a mutex (no id-generator port).
type RequestService struct {
	repo  outbound.RequestRepo
	clock outbound.Clock

	mu  sync.Mutex
	seq int64
}

// NewRequestService wires the service over the request repo and a clock.
func NewRequestService(repo outbound.RequestRepo, clock outbound.Clock) *RequestService {
	return &RequestService{repo: repo, clock: clock}
}

// nextID returns a unique id derived from the clock and a monotonic counter, so
// two submits in the same nanosecond still differ.
func (s *RequestService) nextID(nowNano int64) string {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()
	return fmt.Sprintf("req-%d-%d", nowNano, seq)
}

// Submit records a new pending request. listener and songHint are required
// (surfaced as requests.ErrEmptyListener/ErrEmptyRequest); dedication optional.
// The id and CreatedAt come from the injected clock.
func (s *RequestService) Submit(ctx context.Context, listener, songHint, dedication string) (requests.Request, error) {
	now := s.clock.Now()
	req, err := requests.NewRequest(s.nextID(now.UnixNano()), listener, songHint, dedication, now)
	if err != nil {
		return requests.Request{}, err
	}
	if err := s.repo.Save(ctx, req); err != nil {
		return requests.Request{}, err
	}
	return req, nil
}

// List returns requests ordered by id. status="" returns all; a valid status
// filters; an unknown status returns inbound.ErrBadStatus.
func (s *RequestService) List(ctx context.Context, status string) ([]requests.Request, error) {
	if status != "" && !requests.Status(status).Valid() {
		return nil, inbound.ErrBadStatus
	}
	all, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if status == "" {
		return all, nil
	}
	out := make([]requests.Request, 0, len(all))
	for _, r := range all {
		if string(r.Status) == status {
			out = append(out, r)
		}
	}
	return out, nil
}

// moderate applies fn (a domain transition) to the request with id and persists
// the result. Unknown id → outbound.ErrRequestNotFound (propagated from Get);
// forbidden transition → requests.ErrInvalidTransition (propagated from fn).
func (s *RequestService) moderate(ctx context.Context, id string, fn func(requests.Request) (requests.Request, error)) (requests.Request, error) {
	cur, err := s.repo.Get(ctx, id)
	if err != nil {
		return requests.Request{}, err
	}
	next, err := fn(cur)
	if err != nil {
		return requests.Request{}, err
	}
	if err := s.repo.Save(ctx, next); err != nil {
		return requests.Request{}, err
	}
	return next, nil
}

// Approve moves the request to approved.
func (s *RequestService) Approve(ctx context.Context, id string) (requests.Request, error) {
	now := s.clock.Now()
	return s.moderate(ctx, id, func(r requests.Request) (requests.Request, error) { return r.Approve(now) })
}

// Reject moves the request to rejected.
func (s *RequestService) Reject(ctx context.Context, id string) (requests.Request, error) {
	now := s.clock.Now()
	return s.moderate(ctx, id, func(r requests.Request) (requests.Request, error) { return r.Reject(now) })
}

// MarkPlayed moves the request to played.
func (s *RequestService) MarkPlayed(ctx context.Context, id string) (requests.Request, error) {
	now := s.clock.Now()
	return s.moderate(ctx, id, func(r requests.Request) (requests.Request, error) { return r.MarkPlayed(now) })
}

// compile-time: RequestService satisfies the inbound port.
var _ inbound.RequestService = (*RequestService)(nil)
