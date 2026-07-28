"use client";

import type { Deck, EQBand } from "@/hooks/useDeck";
import { EQ_KILL } from "@/hooks/useDeck";

const BANDS: { band: EQBand; label: string }[] = [
  { band: "high", label: "HI" },
  { band: "mid", label: "MID" },
  { band: "low", label: "LO" },
];

/* 3-band EQ for one deck: vertical faders (−26..+6 dB) plus a kill toggle per
   band. Pure view over the hook's eq state + setEQ; the audio is three
   BiquadFilterNodes in useDeck. Kill drops the band to EQ_KILL (effectively
   silent) and back to 0. */
export function EQ3Knobs({ d, disabled }: { d: Deck; disabled: boolean }) {
  return (
    <div className="eq3" role="group" aria-label="Equalizador 3 bandas">
      {BANDS.map(({ band, label }) => {
        const db = d.eq[band];
        const killed = db <= EQ_KILL;
        return (
          <div key={band} className="eq3-band">
            <input
              className="eq3-fader"
              type="range"
              min={-26}
              max={6}
              step={1}
              value={killed ? -26 : db}
              disabled={disabled}
              onChange={(e) => d.setEQ(band, Number(e.target.value))}
              aria-label={"EQ " + label}
            />
            <button
              className={"eq3-kill" + (killed ? " on" : "")}
              disabled={disabled}
              onClick={() => d.setEQ(band, killed ? 0 : EQ_KILL)}
              title={"Kill " + label}
            >
              {label}
            </button>
          </div>
        );
      })}
    </div>
  );
}
