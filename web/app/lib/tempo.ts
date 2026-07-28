/* browser mirror of domain/djdeck/tempo.go — the numbers are decided by the
   pure Go domain (tested 100%); this is the same arithmetic so the hook can
   apply playbackRate/pitch without a round-trip. Keep in sync with tempo.go. */

export const PITCH_RANGE = 8.0; // ±% (Technics turntable convention)

/* percent → AudioBufferSource playbackRate, clamped to ±PITCH_RANGE */
export function pitchToRate(pct: number): number {
  if (pct > PITCH_RANGE) pct = PITCH_RANGE;
  if (pct < -PITCH_RANGE) pct = -PITCH_RANGE;
  return 1 + pct / 100;
}

/* effective BPM of baseBPM played at rate */
export function rateToBPM(baseBPM: number, rate: number): number {
  return baseBPM * rate;
}

/* what it takes to beat-match deckBPM to masterBPM: playback ratio + the
   equivalent pitch-fader percent (clamped). Returns null when either BPM ≤ 0
   (track not analysed) — mirrors ErrNoBPM. */
export function sync(deckBPM: number, masterBPM: number): { ratio: number; pitchPct: number } | null {
  if (deckBPM <= 0 || masterBPM <= 0) return null;
  const ratio = masterBPM / deckBPM;
  let pitchPct = (ratio - 1) * 100;
  if (pitchPct > PITCH_RANGE) pitchPct = PITCH_RANGE;
  if (pitchPct < -PITCH_RANGE) pitchPct = -PITCH_RANGE;
  return { ratio, pitchPct };
}
