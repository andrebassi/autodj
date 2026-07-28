"use client";

import type { Deck } from "@/hooks/useDeck";
import { SLOT_COUNT } from "@/hooks/useDeck";

/* 8-slot hot-cue grid for one deck. Empty slot: click stores the current
   playhead. Filled slot: click seeks the deck there (recreates the BufferSource
   at the offset). Shift-click (or right-click) clears the slot. Purely a view
   over the hook's hotCues array + setCue/jumpCue/clearCue. */
export function HotCuePad({ d, disabled }: { d: Deck; disabled: boolean }) {
  const fire = (slot: number, e: React.MouseEvent) => {
    if (e.shiftKey) {
      d.clearCue(slot);
      return;
    }
    if (d.hotCues[slot] >= 0) d.jumpCue(slot);
    else d.setCue(slot);
  };

  return (
    <div className="hotcues" role="group" aria-label="Hot cues">
      {Array.from({ length: SLOT_COUNT }, (_, i) => {
        const set = d.hotCues[i] >= 0;
        return (
          <button
            key={i}
            className={"hotcue" + (set ? " set" : "")}
            disabled={disabled}
            onClick={(e) => fire(i, e)}
            onContextMenu={(e) => {
              e.preventDefault();
              d.clearCue(i);
            }}
            title={set ? `Hot cue ${i + 1} — clique salta, shift limpa` : `Hot cue ${i + 1} — clique grava`}
          >
            {i + 1}
          </button>
        );
      })}
    </div>
  );
}
