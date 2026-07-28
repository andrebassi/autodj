"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useDeck, type Deck } from "@/hooks/useDeck";
import { crossfadeGains } from "@/lib/crossfade";

/* orchestrates the dual DJ deck: one shared AudioContext (created lazily on the
   first user gesture — browsers block audio before that), decks A and B, and a
   crossfader whose position feeds djdeck's equal-power curve into both decks'
   GainNodes. Nothing here fetches a catalog; the caller loads tracks by id. */
export type DeckPair = {
  a: Deck;
  b: Deck;
  crossfade: number; // 0 = A full, 1 = B full
  setCrossfade: (pos: number) => void;
  ready: boolean; // AudioContext live (post-gesture)
  arm: () => void; // create/resume the context on a user gesture
};

export function useDeckPair(): DeckPair {
  const ctxRef = useRef<AudioContext | null>(null);
  const [ready, setReady] = useState(false);
  const [crossfade, setCross] = useState(0);

  const a = useDeck(ctxRef);
  const b = useDeck(ctxRef);

  const arm = useCallback(() => {
    if (!ctxRef.current) {
      const AC =
        window.AudioContext ||
        (window as unknown as { webkitAudioContext: typeof AudioContext })
          .webkitAudioContext;
      ctxRef.current = new AC();
    }
    if (ctxRef.current.state === "suspended") void ctxRef.current.resume();
    setReady(true);
  }, []);

  // apply the equal-power curve to both gains whenever the fader or a gain moves
  useEffect(() => {
    const [ga, gb] = crossfadeGains(crossfade);
    const ctx = ctxRef.current;
    if (a.gain) a.gain.gain.setValueAtTime(ga, ctx ? ctx.currentTime : 0);
    if (b.gain) b.gain.gain.setValueAtTime(gb, ctx ? ctx.currentTime : 0);
  }, [crossfade, a.gain, b.gain]);

  const setCrossfade = useCallback((pos: number) => {
    setCross(pos < 0 ? 0 : pos > 1 ? 1 : pos);
  }, []);

  // tear the context down on unmount
  useEffect(() => {
    return () => {
      const ctx = ctxRef.current;
      if (ctx && ctx.state !== "closed") void ctx.close();
      ctxRef.current = null;
    };
  }, []);

  return { a, b, crossfade, setCrossfade, ready, arm };
}
