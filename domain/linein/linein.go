// Package linein models a live audio capture (mic/aux/soundcard) as a PCM,
// mixable source on the local playout bus. It is pure — no I/O, no deps beyond
// the standard library.
//
// A line-in is always shared.SourceLineIn: PCM=true, Mixable=true, but never
// Beatmatch/Waveform/ECAD (the capability matrix already fixes that). It is the
// legitimate *live* PCM source — the opposite of Spotify, whose DRM audio stays
// unmixable. This package models only the arm/gain/duck state; a LineInController
// adapter commands the actual Liquidsoap input. No DRM audio is ever involved.
package linein

import (
	"errors"
	"strings"

	"autodj/domain/shared"
)

// Errors returned when building or configuring a line-in input.
var (
	// ErrBadLevel rejects a gain or duck value outside [0,1].
	ErrBadLevel = errors.New("linein: gain/duck must be in [0,1]")
	// ErrEmptyLabel rejects an input with no label.
	ErrEmptyLabel = errors.New("linein: label must not be empty")
)

// Default levels for a freshly created input.
const (
	// DefaultGain is unity — the capture passes through at full level.
	DefaultGain = 1.0
	// DefaultDuck lowers the music to 60% (i.e. -40%) under the live voice.
	DefaultDuck = 0.6
)

// Source is the fixed SourceKind for every line-in: a live PCM capture. Exposed
// so callers can assert the two-engine matrix without importing shared directly.
const Source = shared.SourceLineIn

// Input is one live capture: its identity, whether it is armed onto the bus, its
// pass-through Gain and the Duck applied to the music underneath it while armed.
type Input struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Armed bool    `json:"armed"`
	Gain  float64 `json:"gain"`
	Duck  float64 `json:"duck"`
}

// NewInput builds a disarmed input at unity gain and the default duck. The label
// must be non-empty; the id is caller-supplied (device/mount name).
func NewInput(id, label string) (Input, error) {
	if strings.TrimSpace(label) == "" {
		return Input{}, ErrEmptyLabel
	}
	return Input{
		ID:    id,
		Label: label,
		Armed: false,
		Gain:  DefaultGain,
		Duck:  DefaultDuck,
	}, nil
}

// Validate checks the label and both levels against their allowed ranges.
func (i Input) Validate() error {
	if strings.TrimSpace(i.Label) == "" {
		return ErrEmptyLabel
	}
	if !validLevel(i.Gain) || !validLevel(i.Duck) {
		return ErrBadLevel
	}
	return nil
}

// Arm returns a copy of the input armed onto the bus.
func (i Input) Arm() Input {
	i.Armed = true
	return i
}

// Disarm returns a copy of the input removed from the bus.
func (i Input) Disarm() Input {
	i.Armed = false
	return i
}

// SetGain returns a copy with the pass-through gain set, or ErrBadLevel if out of
// range.
func (i Input) SetGain(v float64) (Input, error) {
	if !validLevel(v) {
		return i, ErrBadLevel
	}
	i.Gain = v
	return i, nil
}

// SetDuck returns a copy with the music-duck level set, or ErrBadLevel if out of
// range.
func (i Input) SetDuck(v float64) (Input, error) {
	if !validLevel(v) {
		return i, ErrBadLevel
	}
	i.Duck = v
	return i, nil
}

// Mixable reports the fixed matrix answer: a line-in is always a mixable PCM
// source. Provided so the two-engine invariant reads explicitly at call sites.
func (i Input) Mixable() bool {
	return shared.Capabilities(Source).Mixable
}

// validLevel reports whether v is a normalized level in [0,1].
func validLevel(v float64) bool {
	return v >= 0 && v <= 1
}
