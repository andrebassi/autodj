/* Operacional-screen shared types. All catalog DATA now comes from the API
   (/api/v1/ui/operational + /api/v1/library/tracks) via the hooks in hooks/ —
   nothing is hardcoded here. Reference types for the media catalogs live in
   lib/uiContent.ts. */

export type OpCategoryKey =
  /* Apresentação do app: é a TELA DE ENTRADA do painel, e de propósito NÃO tem
     chip na barra — só existe como estado inicial, some no primeiro clique em
     qualquer aba e não volta. Por isso vive só aqui e não no catálogo do
     servidor (`content.OperationalCatalog`), que lista os chips. */
  | "sobre"
  | "instantaneas"
  | "musicas"
  | "spotify"
  | "youtube"
  | "vinhetas"
  | "trilhas"
  | "extras"
  | "comerciais"
  | "programetes"
  | "outros"
  | "oferecimentos"
  | "infocantores"
  | "aniversarios";

import type { MarkerDTO } from "@/lib/api";

/* one media row (Interprete / Título / Duração). id set → enqueueable. */
export type OpMedia = {
  /* real catalog id (present when it came from the backend) — enables enqueue */
  id?: string;
  artist: string;
  title: string;
  dur: string;
  /* state badges: I=intro C=carimbo R=refrão marked */
  markers?: string;
  /* audio source: local file (PCM, waveform/BPM), Spotify or YouTube (streaming) */
  source: "local" | "spotify" | "youtube";
  /* fixed category key (music/jingle/…) — for edit form + reclassify */
  category?: string;
  album?: string;
  /* raw duration (ms) — maps waveform peaks/markers to time in the editor */
  durationMs?: number;
  /* PCM source → waveform + markers allowed; Spotify (DRM) → disabled */
  pcm?: boolean;
  /* analysed BPM (Épico 2); 0 when not yet analysed — SYNC needs it */
  bpm?: number;
  /* já existe um arquivo SEM VOCAL preparado por IA para esta faixa? */
  instrumentalReady?: boolean;
  /* pastas EFETIVAS da faixa: as do operador, ou a do serviço em streaming */
  folders?: string[];
  /* current cue markers (kind + pos_ms + label), PCM tracks only */
  cues?: MarkerDTO[];
  /* Camelot key (analysed, PCM only) — detail popup */
  musicalKey?: string;
  /* spotify:track:… deep link (spotify source) — open on platform */
  spotifyUri?: string;
  /* lifetime play tally */
  playCount?: number;
  /* BPM/key analysis has run (PCM only) */
  analyzed?: boolean;
  /* mixable per two-engine matrix (false for spotify) */
  mixable?: boolean;
  /* direct cover URL (Spotify CDN) — render sem depender do /artwork endpoint */
  artworkUrl?: string;
};

import type { Playable } from "@/lib/types";

/* Playable (deck/fila/histórico ao vivo) → OpMedia p/ o popup de detalhes.
   Provider-agnóstico: mesma peça vale p/ Spotify e local. Campos só-do-catálogo
   (BPM/tom/mixável/análise/marcadores) ficam ausentes aqui — o Playable não os
   carrega — e o TrackDetailModal simplesmente os omite (não polui com "—"). */
export function playableToMedia(p: Playable): OpMedia {
  const isSpot = p.source === "spotify";
  const isYt = p.source === "youtube";
  return {
    id: p.track_id || undefined,
    artist: p.artist && p.artist.trim() !== "" ? p.artist : "—",
    title: p.title || p.track_id || "—",
    dur: "",
    source: isSpot ? "spotify" : isYt ? "youtube" : "local",
    durationMs: p.duration_ms,
    artworkUrl: p.artwork_url || undefined,
    spotifyUri: isSpot ? p.uri : undefined,
  };
}
