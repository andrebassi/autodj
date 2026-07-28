// Package djdeck holds the pure DJ-deck logic the browser Web Audio graph
// consumes: the crossfader curve and the per-deck state machine. It has zero
// external deps — no Web Audio, no I/O. The hook (web/) drives the actual
// AudioContext; this package only decides gains and validates transitions, so
// the beatmatch/crossfade math stays unit-testable at 100%.
//
// Two-engine invariant: a deck only ever loads a Mixable && PCMAvailable track
// (enforced at the app/endpoint layer in 5.1); this package assumes PCM.
package djdeck

import (
	"errors"
	"math"
)

// Crossfade returns the equal-power gains for deck A and B at fader position
// pos in [0,1]. pos=0 → A full / B silent; pos=1 → A silent / B full; pos=0.5 →
// ~0.707 each. Equal-power (constant total *power*, not amplitude) is the DJ
// standard so the perceived loudness stays flat across the blend:
//
//	gainA = cos(pos·π/2)   gainB = sin(pos·π/2)   gainA²+gainB² = 1
//
// pos is clamped to [0,1] so an out-of-range slider can't produce negative or
// >1 gains.
func Crossfade(pos float64) (gainA, gainB float64) {
	if pos < 0 {
		pos = 0
	}
	if pos > 1 {
		pos = 1
	}
	angle := pos * math.Pi / 2
	return math.Cos(angle), math.Sin(angle)
}

// DeckState is the lifecycle of one deck.
type DeckState string

const (
	// StateEmpty has no track loaded.
	StateEmpty DeckState = "empty"
	// StateLoaded has a track decoded but never played, playhead at 0.
	StateLoaded DeckState = "loaded"
	// StateCued is parked at a cue point, ready to fire.
	StateCued DeckState = "cued"
	// StatePlaying is currently playing.
	StatePlaying DeckState = "playing"
	// StatePaused is stopped mid-track, playhead held.
	StatePaused DeckState = "paused"
	// StateEnded reached the end of the track.
	StateEnded DeckState = "ended"
)

// ErrBadTransition is returned when a deck action is invalid for the current
// state (e.g. Play on an Empty deck).
var ErrBadTransition = errors.New("djdeck: invalid deck transition")

// Deck is a pure state machine for one deck. It carries no audio — the hook
// mirrors it against the real AudioContext. Zero value is a valid Empty deck.
type Deck struct {
	state DeckState
}

// State returns the current state (Empty for the zero value).
func (d *Deck) State() DeckState {
	if d.state == "" {
		return StateEmpty
	}
	return d.state
}

// Load decodes a track onto the deck. Valid from any state (loading replaces
// whatever was there). Lands in Loaded.
func (d *Deck) Load() error {
	d.state = StateLoaded
	return nil
}

// Cue parks the deck at a cue point. Valid from Loaded/Playing/Paused/Cued/Ended
// (there must be a track). Invalid from Empty.
func (d *Deck) Cue() error {
	switch d.State() {
	case StateEmpty:
		return ErrBadTransition
	default:
		d.state = StateCued
		return nil
	}
}

// Play starts/resumes playback. Valid from Loaded/Cued/Paused/Playing. Invalid
// from Empty (nothing to play) and Ended (must Cue/Load first).
func (d *Deck) Play() error {
	switch d.State() {
	case StateLoaded, StateCued, StatePaused, StatePlaying:
		d.state = StatePlaying
		return nil
	default:
		return ErrBadTransition
	}
}

// Pause holds the playhead. Valid only from Playing.
func (d *Deck) Pause() error {
	if d.State() != StatePlaying {
		return ErrBadTransition
	}
	d.state = StatePaused
	return nil
}

// End marks the track finished. Valid only from Playing (a track ends while
// playing).
func (d *Deck) End() error {
	if d.State() != StatePlaying {
		return ErrBadTransition
	}
	d.state = StateEnded
	return nil
}

// Eject clears the deck back to Empty. Valid from any state.
func (d *Deck) Eject() error {
	d.state = StateEmpty
	return nil
}
