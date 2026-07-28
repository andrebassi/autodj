// Package waveform implements outbound.WaveformGenerator with a deterministic
// placeholder: the peaks are synthesized from the track id (FNV-1a → mulberry32
// PRNG, envelope random-walk + sparse spikes), so the rendered wave is stable
// across reloads and identical to what the frontend would draw. It stands in for
// the real generator (audiowaveform → peaks JSON) with the same wire shape, so
// the app and UI contract are already frozen. Never receives a DRM track — the
// app layer gates non-PCM upstream.
package waveform

import (
	"context"

	"autodj/domain/library"
	"autodj/ports/outbound"
)

// defaultBars is the peak count used when the caller asks for a non-positive
// number (matches the screen density the editor draws).
const defaultBars = 120

// Placeholder is the deterministic peak generator.
type Placeholder struct{}

// New returns a Placeholder. Compile-time check it satisfies the port.
func New() *Placeholder { return &Placeholder{} }

var _ outbound.WaveformGenerator = (*Placeholder)(nil)

// Peaks returns bars deterministic amplitudes in [0,1] derived from the track
// id. A non-positive bars falls back to defaultBars. The shape is an amplitude
// envelope doing a bounded random walk with occasional transient spikes — a
// believable stand-in for a real waveform until the analyzer feeds real peaks.
func (Placeholder) Peaks(_ context.Context, t *library.Track, bars int) ([]float64, error) {
	if bars <= 0 {
		bars = defaultBars
	}
	seed := fnv1a(t.ID)
	rng := mulberry32(seed)
	out := make([]float64, bars)
	env := 0.5
	for i := 0; i < bars; i++ {
		// Bounded random walk keeps the envelope in a musical mid-band.
		env += (rng() - 0.5) * 0.3
		env = clamp01(env)
		v := env * (0.55 + 0.45*rng())
		// Sparse transient spikes (~6%) give the wave visible peaks.
		if rng() < 0.06 {
			v = clamp01(v + 0.35+0.25*rng())
		}
		out[i] = clamp01(v)
	}
	return out, nil
}

// fnv1a is the 32-bit FNV-1a hash of s (same constants as the JS side).
func fnv1a(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// mulberry32 is the classic tiny PRNG: returns a closure yielding floats in
// [0,1). Deterministic for a given seed.
func mulberry32(seed uint32) func() float64 {
	state := seed
	return func() float64 {
		state += 0x6D2B79F5
		z := state
		z = (z ^ (z >> 15)) * (z | 1)
		z ^= z + (z^(z>>7))*(z|61)
		z ^= z >> 14
		return float64(z) / 4294967296.0
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
