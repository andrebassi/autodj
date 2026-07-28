"use client";

import type { Deck } from "@/hooks/useDeck";

/* pitch fader (±8%, Technics convention) + SYNC. The slider drives the deck's
   playbackRate via the hook (domain clamps). SYNC beat-matches this deck to
   masterBPM (the other deck's effective tempo) — disabled when either BPM is
   unknown (track not analysed). Shows the effective BPM live. */
export function PitchFader({
  d,
  masterBPM,
  onSync,
}: {
  d: Deck;
  masterBPM: number;
  onSync: () => void;
}) {
  const canSync = d.bpm > 0 && masterBPM > 0;
  return (
    <div className="pitch">
      <input
        className="pitch-fader"
        type="range"
        min={-8}
        max={8}
        step={0.1}
        value={d.pitchPct}
        onChange={(e) => d.setPitch(Number(e.target.value))}
        aria-label="Pitch"
      />
      <div className="pitch-info">
        <span className="pitch-pct">
          {d.pitchPct >= 0 ? "+" : ""}
          {d.pitchPct.toFixed(1)}%
        </span>
        <span className="pitch-bpm">{d.effectiveBPM > 0 ? d.effectiveBPM.toFixed(1) : "—"} BPM</span>
      </div>
      <button
        className="pitch-sync"
        disabled={!canSync}
        onClick={onSync}
        title={canSync ? "Casar BPM ao outro deck" : "faixa não analisada (sem BPM)"}
      >
        SYNC
      </button>
    </div>
  );
}
