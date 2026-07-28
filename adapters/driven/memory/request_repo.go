package memory

import (
	"context"
	"sort"
	"sync"

	"autodj/domain/requests"
	"autodj/ports/outbound"
)

// RequestRepo implements outbound.RequestRepo in memory (multi-row, id-keyed). It
// starts empty. Used by the ephemeral repo build and unit tests.
type RequestRepo struct {
	mu   sync.RWMutex
	reqs map[string]requests.Request
}

// NewRequestRepo returns an empty in-memory request repo.
func NewRequestRepo() *RequestRepo {
	return &RequestRepo{reqs: map[string]requests.Request{}}
}

// Save upserts the request by id.
func (r *RequestRepo) Save(_ context.Context, req requests.Request) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqs[req.ID] = req
	return nil
}

// Get returns the request with id, or outbound.ErrRequestNotFound.
func (r *RequestRepo) Get(_ context.Context, id string) (requests.Request, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	req, ok := r.reqs[id]
	if !ok {
		return requests.Request{}, outbound.ErrRequestNotFound
	}
	return req, nil
}

// List returns all requests ordered by id (stable output).
func (r *RequestRepo) List(context.Context) ([]requests.Request, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]requests.Request, 0, len(r.reqs))
	for _, req := range r.reqs {
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
