"use client";

import type { VizMode } from "@/hooks/useVizMode";
import { VuMeter } from "@/components/VuMeter";
import { NowProgress } from "@/components/NowProgress";

/* Faixa de visualização do cabeçalho. Alterna entre a LINHA de progresso real
   (posição/duração da faixa no ar) e o WAVEFORM cosmético. Mantém os dois; o
   botãozinho no canto troca e a escolha persiste (useVizMode). */
export function NowViz({
  mode,
  onCycle,
  active,
  posMs,
  durMs,
}: {
  mode: VizMode;
  onCycle: () => void;
  active: boolean;
  posMs?: number;
  durMs?: number;
}) {
  return (
    <div className="nowviz">
      {mode === "wave" ? (
        <VuMeter active={active} />
      ) : (
        <NowProgress active={active} posMs={posMs} durMs={durMs} />
      )}
      <button
        type="button"
        className="nowviz-toggle"
        onClick={onCycle}
        title={mode === "wave" ? "Mostrar linha de progresso" : "Mostrar waveform"}
        aria-label="Alternar visualização (linha / waveform)"
      >
        {mode === "wave" ? "▬" : "≈"}
      </button>
    </div>
  );
}
