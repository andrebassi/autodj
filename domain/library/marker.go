package library

import (
	"errors"
	"sort"
)

var (
	// ErrMarkersNotPCM is returned when markers are set on a non-PCM (DRM)
	// track. Cue points require a local PCM source — Spotify never gets them.
	ErrMarkersNotPCM = errors.New("track: markers require a PCM source")
	// ErrBadMarkerKind is returned for an unknown MarkerKind.
	ErrBadMarkerKind = errors.New("track: invalid marker kind")
	// ErrMarkerOutOfRange is returned when a marker position is negative or
	// past the track duration.
	ErrMarkerOutOfRange = errors.New("track: marker position out of range")
	// ErrDuplicateMarker is returned when a singleton kind (start/end/intro/
	// mixin/mixout) appears more than once.
	ErrDuplicateMarker = errors.New("track: duplicate singleton marker")
	// ErrMarkerOrder is returned when start is after end, or mixin after mixout.
	ErrMarkerOrder = errors.New("track: marker order invalid (start≤end, mixin≤mixout)")
	// ErrWaveformNotPCM is returned when a waveform is requested for a non-PCM
	// (DRM) track. Peaks require a local PCM source — Spotify never gets a wave.
	ErrWaveformNotPCM = errors.New("track: waveform requires a PCM source")
	// ErrTrackNotMixable is returned when raw audio bytes are requested for a
	// non-mixable (DRM) track. The DJ deck / cartwall only decode PCM — Spotify
	// audio never leaves the SDK, so its bytes are never served. Fifth surface
	// of the two-engine invariant (deck audio endpoint).
	ErrTrackNotMixable = errors.New("track: audio requires a mixable PCM source")
	// ErrNoAudioPath is returned when a mixable track somehow has no file path.
	ErrNoAudioPath = errors.New("track: no audio file path")
)

// MarkerKind names a cue point's role. start/end bound playout; intro is where
// vocals/beat kick in; mixin/mixout are the crossfade anchors; stamp is a
// station-ID overlay point; hook marks a chorus/refrain.
type MarkerKind string

const (
	MarkerStart  MarkerKind = "start"
	MarkerIntro  MarkerKind = "intro"
	MarkerHook   MarkerKind = "hook"
	MarkerStamp  MarkerKind = "stamp"
	MarkerMixIn  MarkerKind = "mixin"
	MarkerMixOut MarkerKind = "mixout"
	MarkerEnd    MarkerKind = "end"
)

// Valid reports whether k is a known marker kind.
func (k MarkerKind) Valid() bool {
	switch k {
	case MarkerStart, MarkerIntro, MarkerHook, MarkerStamp, MarkerMixIn, MarkerMixOut, MarkerEnd:
		return true
	default:
		return false
	}
}

// singleton reports whether a kind may appear at most once per track.
// stamp and hook may repeat; the rest are one-of.
func (k MarkerKind) singleton() bool {
	switch k {
	case MarkerStamp, MarkerHook:
		return false
	default:
		return true
	}
}

// Marker is a cue point on a PCM track: a kind at a millisecond position with
// an optional operator label.
//
// Auto distinguishes what the ANALYZER wrote from what the OPERATOR placed by
// hand. It is the whole reason re-analysis is safe: a new run replaces its own
// marks and never touches the operator's. Without the flag the analyzer would
// have to either overwrite everything (erasing manual work on every re-scan) or
// nothing (leaving stale cues forever).
type Marker struct {
	Kind  MarkerKind
	PosMs int64
	Label string
	Auto  bool
}

// SetMarkers validates and stores cue markers, replacing any existing set.
// It refuses non-PCM sources (ErrMarkersNotPCM) so the two-engine invariant
// holds — DRM tracks never carry cue points. Markers are sorted by position.
// An empty slice clears the markers.
func (t *Track) SetMarkers(ms []Marker) error {
	if !t.PCMAvailable {
		return ErrMarkersNotPCM
	}
	durMs := t.Duration.Milliseconds()
	seen := map[MarkerKind]bool{}
	pos := map[MarkerKind]int64{}
	for _, m := range ms {
		if !m.Kind.Valid() {
			return ErrBadMarkerKind
		}
		if m.PosMs < 0 || m.PosMs > durMs {
			return ErrMarkerOutOfRange
		}
		if m.Kind.singleton() {
			if seen[m.Kind] {
				return ErrDuplicateMarker
			}
			seen[m.Kind] = true
			pos[m.Kind] = m.PosMs
		}
	}
	if s, e := pos[MarkerStart], pos[MarkerEnd]; seen[MarkerStart] && seen[MarkerEnd] && s > e {
		return ErrMarkerOrder
	}
	if in, out := pos[MarkerMixIn], pos[MarkerMixOut]; seen[MarkerMixIn] && seen[MarkerMixOut] && in > out {
		return ErrMarkerOrder
	}
	out := append([]Marker(nil), ms...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].PosMs < out[j].PosMs })
	t.Markers = out
	return nil
}

// markerAt returns the position of the first marker of kind k, or 0 when absent.
// Singleton kinds have at most one, so "first" is "the one".
func (t *Track) markerAt(k MarkerKind) int64 {
	for _, m := range t.Markers {
		if m.Kind == k {
			return m.PosMs
		}
	}
	return 0
}

// CueInMs is where playout should start (the `start` marker), 0 = from the top.
// Reading it here — instead of letting each caller walk the marker slice — keeps
// the cue vocabulary in one place: whether a cue is auto or manual is invisible
// downstream, which is exactly the point (the operator's hand-placed start must
// drive the deck the same way the analyzer's does).
func (t *Track) CueInMs() int64 { return t.markerAt(MarkerStart) }

// CueOutMs is where playout should stop (the `end` marker), 0 = play to the end.
func (t *Track) CueOutMs() int64 { return t.markerAt(MarkerEnd) }

// VocalMs is where the voice comes in (the `intro` marker), 0 = not marked.
// 0 is a real answer, not a missing one: an ambiguous track is left unmarked on
// purpose, and the loop/announce features stay disabled for it.
func (t *Track) VocalMs() int64 { return t.markerAt(MarkerIntro) }

// ApplyAutoMarkers replaces only the ANALYZER's markers, preserving every marker
// the operator placed by hand. A manual singleton always wins: if the operator
// pinned the start himself, a re-analysis does not move it — the machine
// proposes, the operator decides.
//
// Validation/ordering are not re-implemented here: the merged set goes through
// SetMarkers, so the singleton, range and order rules stay in one place. The
// incoming markers are forced to Auto regardless of what the caller set, so an
// analyzer can never smuggle a marker in as manual.
func (t *Track) ApplyAutoMarkers(auto []Marker) error {
	if !t.PCMAvailable {
		return ErrMarkersNotPCM
	}
	merged := make([]Marker, 0, len(t.Markers)+len(auto))
	taken := map[MarkerKind]bool{}
	for _, m := range t.Markers {
		if m.Auto {
			continue // marca velha do analisador: some, a nova entra no lugar
		}
		merged = append(merged, m)
		if m.Kind.singleton() {
			taken[m.Kind] = true
		}
	}
	for _, m := range auto {
		if m.Kind.singleton() && taken[m.Kind] {
			continue // o operador já fixou esta: não sobrescreve
		}
		m.Auto = true
		merged = append(merged, m)
		if m.Kind.singleton() {
			taken[m.Kind] = true
		}
	}
	return t.SetMarkers(merged)
}
