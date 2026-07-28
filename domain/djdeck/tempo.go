package djdeck

import "errors"

// ErrNoBPM is returned when a tempo calculation is asked to work with a
// non-positive BPM — a track that was never analysed (Épico 2) or a DRM source
// that has no PCM to analyse. Beatmatch is impossible without a real BPM.
var ErrNoBPM = errors.New("djdeck: BPM unavailable (track not analysed)")

// PitchRange is the classic turntable pitch range in percent (Technics ±8%).
// Beyond it beatmatch sounds artificial, so PitchToRate clamps to it.
const PitchRange = 8.0

// TempoRatio returns the playback-rate ratio that makes a track at sourceBPM
// beat-match a track at targetBPM: target/source. A ratio > 1 speeds the source
// up, < 1 slows it down. Either BPM ≤ 0 → ErrNoBPM.
func TempoRatio(sourceBPM, targetBPM float64) (float64, error) {
	if sourceBPM <= 0 || targetBPM <= 0 {
		return 0, ErrNoBPM
	}
	return targetBPM / sourceBPM, nil
}

// PitchToRate converts a pitch fader position in percent to an AudioBufferSource
// playbackRate: 1 + pct/100. pct is clamped to [-PitchRange, +PitchRange] so the
// fader can never push tempo past the turntable convention.
func PitchToRate(pct float64) float64 {
	if pct > PitchRange {
		pct = PitchRange
	}
	if pct < -PitchRange {
		pct = -PitchRange
	}
	return 1 + pct/100
}

// RateToBPM returns the effective BPM of a track at baseBPM played at the given
// rate. Used to show the operator the real tempo while the pitch fader moves.
func RateToBPM(baseBPM, rate float64) float64 {
	return baseBPM * rate
}

// Sync computes what it takes to beat-match deckBPM to masterBPM: the playback
// ratio (masterBPM/deckBPM) and the equivalent pitch-fader percent
// ((ratio-1)*100), so the UI can both apply the rate and move the fader. The
// pitch percent is clamped to the fader range — if the two tempos differ by more
// than ±PitchRange the match is partial (as on a real turntable). Either BPM ≤ 0
// → ErrNoBPM.
func Sync(deckBPM, masterBPM float64) (ratio, pitchPct float64, err error) {
	ratio, err = TempoRatio(deckBPM, masterBPM)
	if err != nil {
		return 0, 0, err
	}
	pitchPct = (ratio - 1) * 100
	if pitchPct > PitchRange {
		pitchPct = PitchRange
	}
	if pitchPct < -PitchRange {
		pitchPct = -PitchRange
	}
	return ratio, pitchPct, nil
}
