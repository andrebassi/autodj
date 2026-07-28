// Package shared holds cross-context value objects. The two-engine split
// (Spotify=DRM/no-PCM vs local=own-PCM) is modeled here and is load-bearing:
// it bifurcates capability across the whole domain, not a UI toggle.
package shared

// SourceKind identifies where a Playable's audio comes from. It determines,
// via the capability matrix, whether the audio can be crossfaded, beatmatched,
// analyzed, fed into the local playout/mix bus, or reported to ECAD.
type SourceKind string

const (
	// SourceLocal is a local PCM file — full capability.
	SourceLocal SourceKind = "local"
	// SourceSpotify streams via the Web Playback SDK under DRM/EME. No PCM is
	// ever exposed, so it can never be crossfaded, beatmatched, or mixed into
	// the local playout bus.
	SourceSpotify SourceKind = "spotify"
	// SourceLineIn is a hardware capture (aux/mic/soundcard) — PCM available.
	SourceLineIn SourceKind = "line_in"
	// SourceTTS is synthesized speech (AI DJ / voiceover) — PCM available.
	SourceTTS SourceKind = "tts"
	// SourceYouTube plays via the YouTube IFrame Player API (embed/DRM). Like
	// Spotify, no PCM is ever exposed, so it can never be crossfaded, beatmatched,
	// or mixed into the local playout bus. It plays out-of-band in a visible
	// browser iframe and is swapped by hard cut only.
	SourceYouTube SourceKind = "youtube"
)

// Valid reports whether s is a known SourceKind.
func (s SourceKind) Valid() bool {
	switch s {
	case SourceLocal, SourceSpotify, SourceLineIn, SourceTTS, SourceYouTube:
		return true
	default:
		return false
	}
}

// Transition is how the engine moves from one Playable to the next.
type Transition string

const (
	// TransitionCrossfade overlaps two PCM streams (mixed by Liquidsoap).
	TransitionCrossfade Transition = "crossfade"
	// TransitionFadeTTS ducks the current source, speaks, then brings up the
	// next — the only "mix" available when a Spotify item is involved.
	TransitionFadeTTS Transition = "fade_tts"
	// TransitionHardCut swaps instantly with no overlap.
	TransitionHardCut Transition = "hard_cut"
)

// Capability is the fixed matrix of what each SourceKind can do. It is the
// single source of truth referenced by Marker, Beatgrid, PlayQueue and the
// local playout/mix bus. Changing a row here changes behavior everywhere.
type Capability struct {
	PCM       bool // raw samples available to us
	Crossfade bool // true PCM crossfade possible
	Beatmatch bool // tempo-sync possible (needs PCM + analysis)
	Waveform  bool // waveform/markers meaningful
	Mixable   bool // may enter the local playout/mix bus (needs PCM)
	ECAD      bool // relevant to ECAD royalty reporting
}

var capabilities = map[SourceKind]Capability{
	SourceLocal:   {PCM: true, Crossfade: true, Beatmatch: true, Waveform: true, Mixable: true, ECAD: true},
	SourceSpotify: {PCM: false, Crossfade: false, Beatmatch: false, Waveform: false, Mixable: false, ECAD: false},
	SourceLineIn:  {PCM: true, Crossfade: true, Beatmatch: false, Waveform: false, Mixable: true, ECAD: false},
	SourceTTS:     {PCM: true, Crossfade: true, Beatmatch: false, Waveform: false, Mixable: true, ECAD: false},
	SourceYouTube: {PCM: false, Crossfade: false, Beatmatch: false, Waveform: false, Mixable: false, ECAD: false},
}

// Capabilities returns the capability matrix row for s. Unknown kinds return a
// zero (all-false) Capability — the safe default.
func Capabilities(s SourceKind) Capability {
	return capabilities[s]
}

// AllowedTransitions returns the transitions valid when moving into a Playable
// of the given kind. Spotify (no PCM) can never crossfade.
func AllowedTransitions(s SourceKind) []Transition {
	if Capabilities(s).Crossfade {
		return []Transition{TransitionCrossfade, TransitionFadeTTS, TransitionHardCut}
	}
	return []Transition{TransitionFadeTTS, TransitionHardCut}
}

// DefaultTransition is the standard entry transition for a source: crossfade
// for PCM-mixable sources (Liquidsoap overlaps the streams), hard_cut for
// non-mixable ones (e.g. Spotify) — those play out-of-band (browser SDK) and
// can only be swapped instantly, never mixed into the local PCM bus.
func DefaultTransition(s SourceKind) Transition {
	if Capabilities(s).Crossfade {
		return TransitionCrossfade
	}
	return TransitionHardCut
}

// TransitionAllowed reports whether t may be used moving into kind s.
func TransitionAllowed(s SourceKind, t Transition) bool {
	for _, a := range AllowedTransitions(s) {
		if a == t {
			return true
		}
	}
	return false
}
