"use client";

import { useCallback, useEffect, useState } from "react";
import {
  type CategoryDTO,
  type TrackDTO,
  type TrackForm,
  fetchCategories,
  fetchTracks,
  uploadTrack,
  updateTrack,
  deleteTrack,
} from "@/lib/api";
import type { OpMedia } from "@/lib/operational";

function fmtDur(ms: number): string {
  const s = Math.floor(ms / 1000);
  const hh = String(Math.floor(s / 3600)).padStart(2, "0");
  const mm = String(Math.floor((s % 3600) / 60)).padStart(2, "0");
  const ss = String(s % 60).padStart(2, "0");
  return `${hh}:${mm}:${ss}`;
}

/* short state badge from the cue set: I=intro R=refrão C=carimbo (hook/stamp) */
function markerBadge(cues: TrackDTO["markers"]): string {
  const kinds = new Set((cues ?? []).map((m) => m.kind));
  let b = "";
  if (kinds.has("intro")) b += "I";
  if (kinds.has("hook")) b += "R";
  if (kinds.has("stamp")) b += "C";
  return b;
}

function toMedia(d: TrackDTO): OpMedia {
  return {
    id: d.id,
    artist: d.artist || "—",
    title: d.title,
    album: d.album,
    dur: fmtDur(d.duration_ms),
    source: d.source === "spotify" ? "spotify" : "local",
    category: d.category,
    durationMs: d.duration_ms,
    pcm: d.pcm_available,
    bpm: d.bpm,
    folders: d.folders ?? [],
    cues: d.markers ?? [],
    markers: markerBadge(d.markers),
    musicalKey: d.key,
    spotifyUri: d.spotify_uri,
    playCount: d.play_count,
    analyzed: d.analyzed,
    instrumentalReady: d.instrumental_ready,
    mixable: d.mixable,
  };
}

export type LibraryApi = {
  tracks: OpMedia[];
  loaded: boolean;
  categories: CategoryDTO[];
  reload: () => Promise<void>;
  upload: (file: File, form: TrackForm) => Promise<void>;
  update: (id: string, form: TrackForm) => Promise<void>;
  remove: (id: string) => Promise<void>;
};

/* fetches the real catalog + fixed categories and exposes CRUD mutations.
   Mutations reload the list so the table always mirrors the backend. */
/* ESPELHO LOCAL DA BIBLIOTECA (0.2.27) — lista de faixas em localStorage.

   Só METADADO (título, intérprete, duração, pastas): dezenas de KB, exatamente o
   que localStorage guarda bem. O áudio mora no Cache Storage, que é outra coisa.

   Por que precisa: num F5 sem servidor a lista vinha vazia, e aí a tela decidia
   sozinha que "não há faixa local" e pulava pra aba Spotify — justamente offline,
   onde o Spotify é a única coisa que NÃO funciona. Com o espelho, Músicas continua
   listando o acervo e tocando o que está em cache. */
const MIRROR = "autodj.library.v1";

function readMirror(): OpMedia[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(MIRROR);
    const arr = raw ? (JSON.parse(raw) as OpMedia[]) : [];
    return Array.isArray(arr) ? arr : [];
  } catch {
    return [];
  }
}

export function useLibrary(): LibraryApi {
  const [tracks, setTracks] = useState<OpMedia[]>(() => readMirror());
  const [categories, setCategories] = useState<CategoryDTO[]>([]);
  const [loaded, setLoaded] = useState(false);

  const reload = useCallback(async () => {
    try {
      const dtos = await fetchTracks();
      const media = dtos.map(toMedia);
      setTracks(media);
      try {
        window.localStorage.setItem(MIRROR, JSON.stringify(media));
      } catch {
        /* cota estourada: o app segue, só perde a lista offline */
      }
    } catch {
      /* servidor fora: mantém o espelho já carregado (nunca zera a lista) */
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const cats = await fetchCategories();
        if (!stop) setCategories(cats);
      } catch {
        /* ignore — modal falls back to empty select */
      }
      if (!stop) await reload();
    })();
    return () => {
      stop = true;
    };
  }, [reload]);

  const upload = useCallback(
    async (file: File, form: TrackForm) => {
      await uploadTrack(file, form);
      await reload();
    },
    [reload],
  );

  const update = useCallback(
    async (id: string, form: TrackForm) => {
      await updateTrack(id, form);
      await reload();
    },
    [reload],
  );

  const remove = useCallback(
    async (id: string) => {
      await deleteTrack(id);
      await reload();
    },
    [reload],
  );

  return { tracks, loaded, categories, reload, upload, update, remove };
}
