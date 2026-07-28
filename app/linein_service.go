package app

import (
	"context"
	"sync"

	"autodj/domain/linein"
	"autodj/ports/inbound"
	"autodj/ports/outbound"
)

// LineInService arms live PCM captures (mic/aux/soundcard) onto the local playout
// bus (Story 7.2). Line-in is the legitimate *live* PCM source — always mixable,
// never DRM — so it ducks the music under a voice without breaching the two-engine
// invariant (Spotify stays unmixable; line-in is real PCM).
//
// Inputs are seeded at construction (the operator's configured devices) and held
// in memory in a stable order — single operator, home use, like RDSService's flow.
type LineInService struct {
	ctrl outbound.LineInController // commands the engine; must be non-nil

	mu     sync.Mutex
	order  []string                 // stable id order for List
	inputs map[string]linein.Input
}

// NewLineInService builds the service over a controller and the operator's seed
// inputs, preserving their order. Duplicate ids collapse to the first seen.
func NewLineInService(ctrl outbound.LineInController, seed ...linein.Input) *LineInService {
	s := &LineInService{ctrl: ctrl, inputs: make(map[string]linein.Input)}
	for _, in := range seed {
		if _, dup := s.inputs[in.ID]; dup {
			continue
		}
		s.order = append(s.order, in.ID)
		s.inputs[in.ID] = in
	}
	return s
}

// List reports every configured input in seed order.
func (s *LineInService) List(_ context.Context) []linein.Input {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]linein.Input, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.inputs[id])
	}
	return out
}

// Arm arms the input onto the bus, commanding the controller, and stores it.
// Returns ErrLineInNotFound for an unknown id.
func (s *LineInService) Arm(ctx context.Context, id string) (linein.Input, error) {
	return s.mutate(ctx, id, func(in linein.Input) (linein.Input, error) {
		return in.Arm(), nil
	}, true)
}

// Disarm removes the input from the bus (idempotent). Returns ErrLineInNotFound
// for an unknown id.
func (s *LineInService) Disarm(ctx context.Context, id string) (linein.Input, error) {
	s.mu.Lock()
	in, ok := s.inputs[id]
	s.mu.Unlock()
	if !ok {
		return linein.Input{}, inbound.ErrLineInNotFound
	}
	in = in.Disarm()
	if err := s.ctrl.Disarm(ctx, id); err != nil {
		return linein.Input{}, err
	}
	s.mu.Lock()
	s.inputs[id] = in
	s.mu.Unlock()
	return in, nil
}

// SetGain sets the pass-through gain and pushes new levels when the input is armed.
func (s *LineInService) SetGain(ctx context.Context, id string, v float64) (linein.Input, error) {
	return s.mutate(ctx, id, func(in linein.Input) (linein.Input, error) {
		return in.SetGain(v)
	}, false)
}

// SetDuck sets the music-duck level and pushes new levels when the input is armed.
func (s *LineInService) SetDuck(ctx context.Context, id string, v float64) (linein.Input, error) {
	return s.mutate(ctx, id, func(in linein.Input) (linein.Input, error) {
		return in.SetDuck(v)
	}, false)
}

// mutate applies fn to the input id, then commands the controller: Arm when
// arming; SetLevels when the (possibly already-armed) input needs its levels
// pushed. A disarmed input's level change is stored without touching the bus.
func (s *LineInService) mutate(
	ctx context.Context,
	id string,
	fn func(linein.Input) (linein.Input, error),
	arming bool,
) (linein.Input, error) {
	s.mu.Lock()
	cur, ok := s.inputs[id]
	s.mu.Unlock()
	if !ok {
		return linein.Input{}, inbound.ErrLineInNotFound
	}
	next, err := fn(cur)
	if err != nil {
		return linein.Input{}, err
	}
	switch {
	case arming:
		if err := s.ctrl.Arm(ctx, next); err != nil {
			return linein.Input{}, err
		}
	case next.Armed:
		if err := s.ctrl.SetLevels(ctx, next); err != nil {
			return linein.Input{}, err
		}
	}
	s.mu.Lock()
	s.inputs[id] = next
	s.mu.Unlock()
	return next, nil
}
