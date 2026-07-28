// Package linein implements the outbound.LineInController port. It commands the
// playout engine's live-capture inputs (mic/aux/soundcard) — arm/disarm/levels.
// Line-in is the legitimate *live* PCM source: always mixable, never DRM, the
// opposite of Spotify. The NoopController is the home-use stub — it records the
// last command so the operator UI reflects state without a real audio bus.
package linein

import (
	"context"
	"sync"

	dlinein "autodj/domain/linein"
)

// NoopController satisfies outbound.LineInController without touching hardware.
// It is the default on a box with no capture device wired to Liquidsoap: the REST
// layer and UI work end-to-end (arm/disarm/levels succeed) against in-memory state.
type NoopController struct {
	mu       sync.Mutex
	armed    map[string]dlinein.Input // last armed input by id
	commands int
}

// NewNoopController builds an empty controller.
func NewNoopController() *NoopController {
	return &NoopController{armed: make(map[string]dlinein.Input)}
}

// Arm records the input as routed onto the (virtual) bus.
func (c *NoopController) Arm(_ context.Context, in dlinein.Input) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.armed[in.ID] = in
	c.commands++
	return nil
}

// Disarm drops the input id from the (virtual) bus.
func (c *NoopController) Disarm(_ context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.armed, id)
	c.commands++
	return nil
}

// SetLevels updates the levels of an already-armed input.
func (c *NoopController) SetLevels(_ context.Context, in dlinein.Input) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.armed[in.ID] = in
	c.commands++
	return nil
}

// Armed reports the last recorded input for id and whether it is on the bus
// (exposed for tests/inspection).
func (c *NoopController) Armed(id string) (dlinein.Input, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	in, ok := c.armed[id]
	return in, ok
}
