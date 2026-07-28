package memory

import (
	"context"
	"sort"
	"sync"

	"autodj/domain/schedule"
	"autodj/ports/outbound"
)

// EventRepo implements outbound.EventRepo in memory (multi-row, id-keyed). It
// starts empty; the wiring seeds schedule.DefaultEvents() when appropriate. Used
// by the ephemeral repo build and unit tests.
type EventRepo struct {
	mu     sync.RWMutex
	events map[string]schedule.ScheduledEvent
}

// NewEventRepo returns an empty in-memory event repo.
func NewEventRepo() *EventRepo {
	return &EventRepo{events: map[string]schedule.ScheduledEvent{}}
}

// List returns all events ordered by id (stable output).
func (r *EventRepo) List(context.Context) ([]schedule.ScheduledEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]schedule.ScheduledEvent, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get returns the event with id, or outbound.ErrEventNotFound.
func (r *EventRepo) Get(_ context.Context, id string) (schedule.ScheduledEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.events[id]
	if !ok {
		return schedule.ScheduledEvent{}, outbound.ErrEventNotFound
	}
	return e, nil
}

// Save upserts the event by id.
func (r *EventRepo) Save(_ context.Context, e schedule.ScheduledEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[e.ID] = e
	return nil
}

// Delete removes the event by id, or returns outbound.ErrEventNotFound.
func (r *EventRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.events[id]; !ok {
		return outbound.ErrEventNotFound
	}
	delete(r.events, id)
	return nil
}
