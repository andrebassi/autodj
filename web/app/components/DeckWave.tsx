"use client";

import { useMemo } from "react";
import type { VizMode } from "@/hooks/useVizMode";

/* Waveform COSMÉTICO do deck (viz "wave"). Spotify é DRM — browser não lê PCM, então
   a onda é ilustrativa (barras determinísticas por deck), MAS o preenchimento até o
   playhead segue a posição REAL (frac). Clicável/arrastável p/ seek quando seekable.
   Waveform real por PCM só p/ arquivos locais (Épico 2). */
const N = 56; // nº de barras

// alturas determinísticas (0.2..1) — pseudo-random estável por deck, sem Math.random.
function bars(deck: "A" | "B"): number[] {
  const seed = deck === "A" ? 17 : 41;
  return Array.from({ length: N }, (_, i) => {
    const v = Math.sin(i * 0.7 + seed) * 0.5 + Math.sin(i * 1.9 + seed * 0.3) * 0.5;
    return 0.28 + Math.abs(v) * 0.72;
  });
}

export function DeckWave({
  deck,
  frac,
  live,
  seekable,
  onDown,
  onMove,
}: {
  deck: "A" | "B";
  frac: number;
  live: boolean;
  seekable: boolean;
  onDown: (e: React.MouseEvent<HTMLDivElement>) => void;
  onMove: (e: React.MouseEvent<HTMLDivElement>) => void;
}) {
  const heights = useMemo(() => bars(deck), [deck]);
  const played = Math.max(0, Math.min(1, frac)) * N;

  return (
    <div
      className={"deck-wave deck-" + deck.toLowerCase() + (seekable ? " seekable" : "")}
      onMouseDown={onDown}
      onMouseMove={onMove}
      title={seekable ? "Clique/arraste p/ avançar ou retroceder" : undefined}
    >
      {heights.map((h, i) => (
        <span
          key={i}
          className={"dw-bar" + (live && i < played ? " on" : "")}
          style={{ height: `${Math.round(h * 100)}%` }}
        />
      ))}
      {live && frac > 0 && (
        <span className="dw-head" style={{ left: `${Math.max(0, Math.min(1, frac)) * 100}%` }} />
      )}
    </div>
  );
}

// re-export só p/ manter VizMode importável de perto (evita import extra em callers)
export type { VizMode };
