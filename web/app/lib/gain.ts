/* Ganho por faixa (normalização de loudness) — LÓGICA PURA.

   Mora num módulo separado pela mesma razão de `lib/version.ts`: o `web/` não
   tem test runner, então tudo que é regra (e não React) fica em função pura, com
   o raciocínio escrito aqui e conferível a olho.

   O PROBLEMA: um MP3 masterizado hoje sai muito mais alto que um rip antigo. Sem
   correção, o operador mexe no volume a cada virada de faixa.

   A SOLUÇÃO: cada faixa carrega seu loudness integrado medido (EBU R128, do
   ffmpeg). A distância entre ela e o alvo do operador vira um fator de volume.

   ⚠️ POR QUE NÃO WEB AUDIO. O caminho "certo" seria rotear o <audio> por um
   GainNode. Não dá: o `AudioContext` nasce `suspended` e exige gesto do usuário
   — e este app tem histórico documentado de autoplay barrado (0.2.26/0.2.27).
   Rotear o áudio por um nó que pode não destravar significa risco de ficar MUDO
   até alguém clicar. Regressão inaceitável num playout.
   Então o ganho entra no `<audio>.volume`, exatamente como o motor do Spotify já
   faz com `SPOT_TARGET = 0.8`. Precedente pronto, e o coordenador de mixagem é
   agnóstico de ganho por desenho (ele rampa ATÉ o alvo que este módulo dá).

   ⚠️ LIMITE HONESTO: `<audio>.volume` não passa de 1. Como a referência é 0.8,
   sobra só ~+1,9 dB de reforço — faixa MUITO mais baixa que o alvo continua mais
   baixa que as outras. Atenuar é ilimitado; amplificar, não. É o preço de não
   usar Web Audio, e está registrado em docs/pendencias.md. */

/** Referência com normalização LIGADA: deixa margem pra amplificar (0.8 → +1,9dB). */
export const NORMALIZE_REF = 0.8;
/** Ganho mínimo: abaixo disto a faixa some do ar. Medida corrompida não emudece a estação. */
export const MIN_GAIN = 0.05;

/* Régua do alvo de loudness — ESPELHO de `domain/mixing.TargetLUFS` (Go).
   Mexeu num, mexa no outro: o servidor decide o alvo conceitual e a UI precisa
   MOSTRAR o mesmo número que o playout aplica, senão o ⚙ mente sobre o efeito.
   89 (o default histórico) ⇒ ≈ −13,3 LUFS, o padrão de streaming. */
export const MIN_TARGET_LUFS = -24;
export const MAX_TARGET_LUFS = -12;

export function targetLUFSFromVol(vol: number): number {
  const v = clamp(vol, 0, 100);
  return MIN_TARGET_LUFS + (v / 100) * (MAX_TARGET_LUFS - MIN_TARGET_LUFS);
}

/** Rótulo do alvo pro ⚙ ("89% ≈ −13,3 LUFS") — knob escondido é knob que mente. */
export function targetLabel(vol: number): string {
  const t = targetLUFSFromVol(vol);
  return `${Math.round(vol)}% ≈ −${Math.abs(t).toFixed(1).replace(".", ",")} LUFS`;
}

export type GainInput = {
  /** loudness integrado medido da faixa (LUFS). 0 = não medido. */
  loudnessLUFS: number;
  /** alvo do operador (LUFS), derivado do NormalizeVol no domínio. */
  targetLUFS: number;
  /** normalização ligada no ⚙ Configurações. */
  normalize: boolean;
};

/* trackGain devolve o volume [0,1] para o `<audio>` desta faixa.

   Desligado ⇒ 1.0 = EXATAMENTE o comportamento de hoje, sem surpresa nenhuma.
   Ligado mas sem medição ⇒ a referência (0.8): a faixa não medida não pode ficar
   gritando por cima das medidas só porque a análise ainda não rodou nela. */
export function trackGain({ loudnessLUFS, targetLUFS, normalize }: GainInput): number {
  if (!normalize) return 1;
  if (!isFinite(loudnessLUFS) || loudnessLUFS === 0) return NORMALIZE_REF;
  // dB → fator linear de amplitude: 20·log10(f) = ΔdB.
  const factor = Math.pow(10, (targetLUFS - loudnessLUFS) / 20);
  return clamp(NORMALIZE_REF * factor, MIN_GAIN, 1);
}

/* gainLabelDB é o que o deck mostra ("−3,2 dB"): quanto o ganho aplicado desvia
   do ganho pleno DAQUELA configuração. Devolve null quando não há o que dizer —
   normalização desligada, ou desvio pequeno demais pra valer um selo na tela. */
export function gainLabelDB(gain: number, normalize: boolean): string | null {
  if (!normalize) return null;
  const db = 20 * Math.log10(gain / NORMALIZE_REF);
  if (!isFinite(db) || Math.abs(db) < 0.5) return null;
  // pt-BR: vírgula decimal, e o sinal explícito (o "+" some no toFixed).
  const txt = Math.abs(db).toFixed(1).replace(".", ",");
  return `${db < 0 ? "−" : "+"}${txt} dB`;
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v));
}
