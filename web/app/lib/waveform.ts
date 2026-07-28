/* Waveform de render (VirtualDJ-style) — forma determinística por seed (track_id).
   NÃO é áudio real: placeholder visual estável até o analyzer (audiowaveform →
   peaks JSON, Épico 2/3) anexar os picos reais ao payload da Track. Determinístico
   p/ hidratar igual no server e client e não "piscar" a cada render. */
export function fakeWave(seed: string, bars = 120): number[] {
  // hash FNV-1a → estado do PRNG mulberry32
  let h = 2166136261 >>> 0;
  for (let i = 0; i < seed.length; i++) {
    h ^= seed.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  let a = h >>> 0;
  const rnd = () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
  const out: number[] = [];
  let env = 0.5; // envelope suave (random-walk) + picos esparsos = cara de música
  for (let i = 0; i < bars; i++) {
    env += (rnd() - 0.5) * 0.35;
    env = Math.max(0.14, Math.min(1, env));
    const spike = rnd() < 0.1 ? rnd() * 0.55 : 0;
    out.push(Math.min(1, env * (0.72 + rnd() * 0.28) + spike));
  }
  return out;
}
