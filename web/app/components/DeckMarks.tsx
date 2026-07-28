"use client";

import { gainLabelDB } from "@/lib/gain";

/* Marcas de análise sobre a barra do deck — o que o playout está fazendo com a
   faixa, visível em vez de mágico.

   Desenha quatro coisas, todas posicionadas por `left: %` sobre a barra:
   • zonas APAGADAS antes do cueIn e depois do cueOut (o trecho que o trim corta);
   • tick 🎤 na entrada do vocal;
   • faixa sombreada do LOOP (do cueIn até o vocal), pulsando quando armado;
   • selo do ganho ("−3,2 dB") quando a faixa está sendo atenuada.

   Por que funciona sem tocar no layout: `.deck-prog` e `.deck-wave` já são
   `position: relative` com filhos absolutos por `left: %` (é como o playhead
   existe) — mesma gramática do `.mk-flag` do editor de marcadores. As marcas
   entram como irmãos, não mudam a caixa.

   As dicas chegam pelo `Playable` que o DeckPair já tem em mãos: nenhum fetch
   novo, e por tabela elas funcionam offline (o snapshot é espelhado em
   localStorage). Fonte não-PCM (Spotify/YouTube) vem com tudo zero, então este
   componente simplesmente não desenha nada — o invariante
   `spotify|youtube ⇒ !mixable` segue intocado. */
export type DeckMarksProps = {
  durMs: number; // duração REAL do arquivo (a régua da barra)
  cueInMs?: number; // início do áudio (0 = sem corte na frente)
  cueOutMs?: number; // fim do áudio (0 = sem corte no fim)
  vocalMs?: number; // entrada do vocal (0 = não marcado)
  loopArmed?: boolean; // LOOP segurando a base agora
  gain?: number; // ganho aplicado [0,1]
  normalize?: boolean; // normalização ligada (sem ela não há selo de ganho)
};

export function DeckMarks({
  durMs,
  cueInMs = 0,
  cueOutMs = 0,
  vocalMs = 0,
  loopArmed = false,
  gain = 1,
  normalize = false,
}: DeckMarksProps) {
  if (durMs <= 0) return null;
  const pct = (ms: number) => Math.max(0, Math.min(100, (ms / durMs) * 100));

  const headCut = cueInMs > 0 ? pct(cueInMs) : 0;
  // 0 e "igual à duração" são o mesmo caso: nada a cortar no fim.
  const tailStart = cueOutMs > 0 && cueOutMs < durMs ? pct(cueOutMs) : 100;
  const vocal = vocalMs > 0 ? pct(vocalMs) : -1;
  // a base do loop é o trecho instrumental: do início do áudio até a voz entrar.
  const showLoop = vocalMs > 0 && vocalMs > cueInMs;
  const gainTxt = gainLabelDB(gain, normalize);

  return (
    <span className="deck-marks" aria-hidden>
      {headCut > 0 && (
        <span className="deck-mark-cut" style={{ left: 0, width: `${headCut}%` }} />
      )}
      {tailStart < 100 && (
        <span
          className="deck-mark-cut"
          style={{ left: `${tailStart}%`, width: `${100 - tailStart}%` }}
        />
      )}
      {showLoop && (
        <span
          className={"deck-mark-loop" + (loopArmed ? " armed" : "")}
          style={{ left: `${headCut}%`, width: `${Math.max(0, vocal - headCut)}%` }}
        />
      )}
      {vocal >= 0 && (
        <span className="deck-mark-vocal" style={{ left: `${vocal}%` }}>
          <span className="deck-mark-ico">🎤</span>
        </span>
      )}
      {gainTxt && <span className="deck-mark-gain">{gainTxt}</span>}
    </span>
  );
}
