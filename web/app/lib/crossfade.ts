/* Equal-power crossfade curve — mirrors Go `domain/djdeck.Crossfade`.
   pos clamps to [0,1]: pos=0 → A full / B silent, pos=1 → inverse,
   pos=0.5 → ~0.707/0.707 (power sum ≈ 1). Kept identical to the domain so
   the deck's audible crossfade matches the tested reference curve. */
export function crossfadeGains(pos: number): [gainA: number, gainB: number] {
  if (pos < 0) pos = 0;
  if (pos > 1) pos = 1;
  const angle = (pos * Math.PI) / 2;
  return [Math.cos(angle), Math.sin(angle)];
}
