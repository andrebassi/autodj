"use client";

import { useMemo } from "react";
import { useDeckPair } from "@/hooks/useDeckPair";
import { useLibrary } from "@/hooks/useLibrary";
import { DjDeck } from "@/components/DjDeck";
import { Crossfader } from "@/components/Crossfader";

/* interactive dual DJ deck (Story 5.1): two Web Audio decks + crossfader.
   Catalog comes from the real library API, filtered to mixable/PCM local tracks
   only — Spotify/DRM never appears here (two-engine invariant). The AudioContext
   is armed on the first pick/play gesture (browser autoplay policy). */
export function DjDeckPair() {
  const pair = useDeckPair();
  const { tracks, loaded } = useLibrary();

  // only mixable PCM local tracks can be decoded/mixed
  const mixable = useMemo(
    () => tracks.filter((t) => t.id && t.source === "local" && t.pcm),
    [tracks],
  );

  return (
    <section className="djpair">
      <div className="djpair-decks">
        <DjDeck
          deck="A"
          d={pair.a}
          tracks={mixable}
          ready={pair.ready}
          onArm={pair.arm}
          masterBPM={pair.b.effectiveBPM}
        />
        <DjDeck
          deck="B"
          d={pair.b}
          tracks={mixable}
          ready={pair.ready}
          onArm={pair.arm}
          masterBPM={pair.a.effectiveBPM}
        />
      </div>

      <Crossfader pos={pair.crossfade} onChange={pair.setCrossfade} />

      {loaded && mixable.length === 0 && (
        <p className="djpair-hint">
          Nenhuma faixa mixável na biblioteca. Escaneie uma pasta com áudio local.
        </p>
      )}
    </section>
  );
}
