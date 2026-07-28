"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchYouTubeStatus,
  youtubeConnect,
  youtubeDisconnect,
  youtubeImport,
  youtubeImportLinks,
  youtubeSearch,
  YouTubeNotConfigured,
  type YouTubeBackend,
  type YouTubeItem,
  type YouTubeKind,
  type YouTubeStatus,
} from "@/lib/api";
import { humanizeThrown } from "@/lib/errors";

const EMPTY: YouTubeStatus = {
  connected: false,
  premium: false,
  freeSearch: false,
  oauth: false,
};

/* espelho fiel do useSpotify. Poll GET /api/v1/youtube/status e expõe
   connect()/disconnect(). connect() navega o browser p/ a authorize URL do Google
   (auth-code com secret no backend); o callback redireciona de volta e o próximo
   poll reflete a conta vinculada. `configured` é false quando o backend não tem as
   3 vars AUTODJ_YOUTUBE_* (rotas 404), então o painel mostra "integração não
   configurada". `intervalMs` = cadência do refresh (default 4s) p/ o status ficar
   vivo após o round-trip do OAuth. */
export function useYouTube(intervalMs = 4000): {
  status: YouTubeStatus;
  configured: boolean;
  loaded: boolean;
  busy: boolean;
  error: string | null;
  connect: () => Promise<void>;
  disconnect: () => Promise<void>;
  results: YouTubeItem[];
  searching: boolean;
  imported: number | null;
  search: (
    query: string,
    kind: YouTubeKind,
    backend?: YouTubeBackend,
  ) => Promise<void>;
  searchAll: (query: string, backend?: YouTubeBackend) => Promise<void>;
  importItem: (item: YouTubeItem) => Promise<void>;
  importLinks: (text: string) => Promise<number>;
} {
  const [status, setStatus] = useState<YouTubeStatus>(EMPTY);
  const [configured, setConfigured] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [results, setResults] = useState<YouTubeItem[]>([]);
  const [searching, setSearching] = useState(false);
  const [imported, setImported] = useState<number | null>(null);

  useEffect(() => {
    let stop = false;
    const load = async () => {
      try {
        const s = await fetchYouTubeStatus();
        if (stop) return;
        if (s === YouTubeNotConfigured) {
          setConfigured(false);
          setStatus(EMPTY);
        } else {
          setConfigured(true);
          setStatus(s);
        }
      } catch {
        /* backend offline → keep last/empty status */
      } finally {
        if (!stop) setLoaded(true);
      }
    };
    load();
    const id = setInterval(load, intervalMs);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, [intervalMs]);

  const connect = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const url = await youtubeConnect();
      window.location.href = url; // navigate to Google authorize (auth-code)
    } catch (e) {
      setError(humanizeThrown(e, "erro ao conectar YouTube"));
      setBusy(false);
    }
  }, []);

  const disconnect = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      await youtubeDisconnect();
      setStatus(EMPTY);
    } catch (e) {
      setError(humanizeThrown(e, "erro ao desconectar YouTube"));
    } finally {
      setBusy(false);
    }
  }, []);

  /* `backend` escolhe o motor da busca: "free" (InnerTube, sem login, sem cota) ou
     "api" (Data API, exige conta). Default "free" — cliente que não passa nada usa o
     caminho barato. Quem decide é o painel; o hook só repassa. */
  const search = useCallback(
    async (
      query: string,
      kind: YouTubeKind,
      backend: YouTubeBackend = "free",
    ): Promise<void> => {
      if (query.trim() === "") {
        setResults([]);
        return;
      }
      setSearching(true);
      setError(null);
      setImported(null);
      try {
        setResults(await youtubeSearch(query, kind, backend));
      } catch (e) {
        setError(humanizeThrown(e, "erro na busca do YouTube"));
        setResults([]);
      } finally {
        setSearching(false);
      }
    },
    [],
  );

  /* busca vídeos E playlists de uma vez e mescla (vídeos primeiro). allSettled: se
     um tipo falhar (quota/limite), o outro ainda aparece. Alimenta a busca
     automática ao digitar — sem escolher tipo. */
  const searchAll = useCallback(
    async (query: string, backend: YouTubeBackend = "free"): Promise<void> => {
      if (query.trim() === "") {
        setResults([]);
        return;
      }
      setSearching(true);
      setError(null);
      setImported(null);
      try {
        const [videos, playlists] = await Promise.allSettled([
          youtubeSearch(query, "video", backend),
          youtubeSearch(query, "playlist", backend),
        ]);
        const ok = (r: PromiseSettledResult<YouTubeItem[]>): YouTubeItem[] =>
          r.status === "fulfilled" ? r.value : [];
        const merged = [...ok(videos), ...ok(playlists)];
        setResults(merged);
        if (merged.length === 0 && videos.status === "rejected") {
          setError(humanizeThrown(videos.reason, "erro na busca do YouTube"));
        }
      } catch (e) {
        setError(humanizeThrown(e, "erro na busca do YouTube"));
        setResults([]);
      } finally {
        setSearching(false);
      }
    },
    [],
  );

  /* imports one catalog item: a playlist expands to its videos, a video imports
     alone. Result count feeds the "importado" feedback. */
  const importItem = useCallback(async (item: YouTubeItem): Promise<void> => {
    setBusy(true);
    setError(null);
    setImported(null);
    try {
      const req =
        item.kind === "playlist"
          ? { playlistID: item.uri }
          : { uris: [item.uri] };
      const tracks = await youtubeImport(req);
      setImported(tracks.length);
    } catch (e) {
      setError(humanizeThrown(e, "erro ao importar do YouTube"));
    } finally {
      setBusy(false);
    }
  }, []);

  /* importa links colados (";", vírgula ou quebra de linha) SEM exigir conta
     conectada — o backend resolve os ids com a API key (1 unidade por lote).
     Devolve quantas faixas entraram (pode ser menos que o colado: vídeo
     não-incorporável é descartado pelo catálogo). */
  const importLinks = useCallback(async (text: string): Promise<number> => {
    setBusy(true);
    setError(null);
    setImported(null);
    try {
      const tracks = await youtubeImportLinks(text);
      setImported(tracks.length);
      return tracks.length;
    } catch (e) {
      setError(humanizeThrown(e, "erro ao importar links do YouTube"));
      return 0;
    } finally {
      setBusy(false);
    }
  }, []);

  return {
    status,
    configured,
    loaded,
    busy,
    error,
    connect,
    disconnect,
    results,
    searching,
    imported,
    search,
    searchAll,
    importItem,
    importLinks,
  };
}
