package library

import "errors"

var (
	// ErrNotAnalyzable is returned when analysis is applied to a non-PCM track.
	// Spotify (DRM) audio has no PCM to analyze, so BPM/key/waveform never exist
	// for it — this guards the two-engine invariant at the domain edge.
	ErrNotAnalyzable = errors.New("track: source is not analyzable (non-PCM)")
	// ErrBadBPM is returned for a negative BPM.
	ErrBadBPM = errors.New("track: bpm must be non-negative")
)

// Analysis is the result of out-of-process audio analysis of a PCM file: the
// tempo, musical key (Camelot notation), a storage ref to the precomputed
// waveform peaks, integrated loudness and the playout cues (where the audio
// actually starts/ends and where the vocal comes in). Any field may be
// zero-valued when a given analyzer tool is absent (graceful degradation) — an
// empty key/waveform never overwrites a previously computed one.
type Analysis struct {
	BPM          float64
	MusicalKey   string  // Camelot notation, e.g. "8A"; "" when unknown
	WaveformRef  string  // storage ref to peaks JSON; "" when unknown
	LoudnessLUFS float64 // integrated loudness (EBU R128); 0 when unknown

	// CueInMs is where the audio really begins (end of the leading silence), and
	// CueOutMs where it really ends (start of the trailing silence). 0 means "no
	// trim" — the file starts/ends with audio, or silence detection did not run.
	CueInMs  int64
	CueOutMs int64
	// VocalMs is where the voice comes in, in milliseconds. 0 = NOT MARKED, which
	// is the deliberate answer whenever the signal is ambiguous: a wrong vocal
	// mark makes the operator talk over the singing, so silence beats a guess.
	VocalMs int64
}

// autoMarkers converts the analysis cues into the marker set the analyzer owns,
// discarding anything that would not survive SetMarkers. A zero position yields
// no marker — 0 means "nothing detected", never "at the very start" (a track
// whose audio starts at 0 needs no trim marker anyway).
//
// A cue past durMs, or an end before the start, is a rounding artifact between
// two separate probes of the same file (ffprobe gives the duration, the analyzer
// gives the cues). Dropping that one cue beats losing the whole analysis — BPM
// and loudness are still good, and the next run may land inside the range.
func (a Analysis) autoMarkers(durMs int64) []Marker {
	inRange := func(pos int64) bool { return pos > 0 && pos <= durMs }
	var out []Marker
	if inRange(a.CueInMs) {
		out = append(out, Marker{Kind: MarkerStart, PosMs: a.CueInMs, Label: "Início do áudio", Auto: true})
	}
	if inRange(a.VocalMs) {
		out = append(out, Marker{Kind: MarkerIntro, PosMs: a.VocalMs, Label: "Entrada do vocal", Auto: true})
	}
	// start ≤ end is a SetMarkers invariant: an out-of-order pair would reject the
	// whole set, so the end goes only when it is really after the start.
	if inRange(a.CueOutMs) && a.CueOutMs >= a.CueInMs {
		out = append(out, Marker{Kind: MarkerEnd, PosMs: a.CueOutMs, Label: "Fim do áudio", Auto: true})
	}
	return out
}

// ApplyAnalysis writes analysis results onto a PCM track and re-derives the
// capability flags via Validate so the two-engine invariants stay intact. It
// refuses non-PCM sources (ErrNotAnalyzable) and negative BPM (ErrBadBPM).
// Empty MusicalKey/WaveformRef and a zero LoudnessLUFS do not
// clobber existing values, so a degraded run (one tool missing) never erases
// prior analysis.
// The cues (in/out/vocal) land as AUTO markers via ApplyAutoMarkers, which
// preserves whatever the operator placed by hand.
func (t *Track) ApplyAnalysis(a Analysis) error {
	if !t.PCMAvailable {
		return ErrNotAnalyzable
	}
	if a.BPM < 0 {
		return ErrBadBPM
	}
	t.BPM = a.BPM
	if a.MusicalKey != "" {
		t.MusicalKey = a.MusicalKey
	}
	if a.WaveformRef != "" {
		t.WaveformRef = a.WaveformRef
	}
	if a.LoudnessLUFS != 0 {
		t.LoudnessLUFS = a.LoudnessLUFS
	}
	if err := t.ApplyAutoMarkers(a.autoMarkers(t.Duration.Milliseconds())); err != nil {
		return err
	}
	return t.Validate()
}
