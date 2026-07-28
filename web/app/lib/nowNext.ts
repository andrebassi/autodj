import type { Playable, QueueItem } from "@/lib/types";

/* Fonte única do "no ar / próxima": header e os decks A/B leem daqui, sem
   divergir. Instalação limpa: sem now-playing real (silêncio / biblioteca vazia)
   → estado ocioso vazio, sem faixa fictícia. Quando o servidor manda np/queue
   reais, reflete eles. */
export type DeckInfo = {
  artist: string;
  title: string;
  full: string;
  durMs: number;
  trackId: string; // p/ derivar a capa (trackArtworkURL); "" quando ocioso
  artworkUrl: string; // capa direta (CDN) vinda do Playable; "" quando ocioso/local
  source: string; // motor da faixa (spotify/youtube/local…) — badge de origem no deck
};

// Ocioso = travessão "—" no header e nos decks (não "nada no ar"): silêncio é um
// estado neutro, não um rótulo. O deck já força "—" quando não live; o header lê
// `full`, então este também precisa ser "—" p/ casarem.
const IDLE_NOW: DeckInfo = {
  artist: "—",
  title: "",
  full: "—",
  durMs: 0,
  trackId: "",
  artworkUrl: "",
  source: "",
};
const IDLE_NEXT: DeckInfo = {
  artist: "—",
  title: "",
  full: "—",
  durMs: 0,
  trackId: "",
  artworkUrl: "",
  source: "",
};

function split(name: string): [string, string] {
  const i = name.indexOf(" — ");
  return i >= 0 ? [name.slice(0, i), name.slice(i + 3)] : [name, ""];
}
// Prefere o campo `artist` real do Playable (snapshot do catálogo, uniforme p/
// todo provider). Legado sem artist cai no split do título ("Artista — Título").
function info(p: Playable): DeckInfo {
  const art = p.artwork_url ?? "";
  if (p.artist && p.artist.trim() !== "") {
    return { artist: p.artist, title: p.title, full: p.title, durMs: p.duration_ms, trackId: p.track_id, artworkUrl: art, source: p.source };
  }
  const [a, t] = split(p.title);
  return { artist: a, title: t, full: p.title, durMs: p.duration_ms, trackId: p.track_id, artworkUrl: art, source: p.source };
}

export function nowNext(
  hasNow: boolean,
  np: Playable | undefined,
  queue: QueueItem[],
): { now: DeckInfo; next: DeckInfo; hasNext: boolean; live: boolean } {
  // Deck B = SÓ a próxima real (queue[1]). Sem fallback p/ queue[0]: com 1 item
  // na fila, Deck A toca e Deck B fica ocioso — não duplica a mesma faixa.
  const nextItem = queue[1];
  return {
    now: hasNow && np ? info(np) : IDLE_NOW,
    next: nextItem ? info(nextItem.playable) : IDLE_NEXT,
    hasNext: !!nextItem,
    live: hasNow,
  };
}
