"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchSpotifyStatus,
  spotifyConnect,
  spotifyDisconnect,
  spotifyImport,
  spotifySearch,
  SpotifyNotConfigured,
  type CatalogItem,
  type SpotifyKind,
  type SpotifyStatus,
} from "@/lib/api";
import { humanizeThrown } from "@/lib/errors";

const EMPTY: SpotifyStatus = { connected: false, premium: false };

/* polls GET /api/v1/spotify/status and exposes connect()/disconnect().
   connect() navigates the browser to the accounts.spotify.com authorize URL
   (PKCE); the callback redirects back and the next poll reflects the linked
   account. `configured` is false when the backend has no client id (routes 404),
   so the panel can show "integração não configurada". `intervalMs` sets the
   refresh cadence (default 4s) so status stays live after the OAuth round-trip. */
export function useSpotify(intervalMs = 4000): {
  status: SpotifyStatus;
  configured: boolean;
  loaded: boolean;
  busy: boolean;
  error: string | null;
  connect: () => Promise<void>;
  disconnect: () => Promise<void>;
  results: CatalogItem[];
  searching: boolean;
  imported: number | null;
  search: (query: string, kind: SpotifyKind) => Promise<void>;
  searchAll: (query: string) => Promise<void>;
  importItem: (item: CatalogItem) => Promise<void>;
} {
  const [status, setStatus] = useState<SpotifyStatus>(EMPTY);
  const [configured, setConfigured] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [results, setResults] = useState<CatalogItem[]>([]);
  const [searching, setSearching] = useState(false);
  const [imported, setImported] = useState<number | null>(null);

  useEffect(() => {
    let stop = false;
    const load = async () => {
      try {
        const s = await fetchSpotifyStatus();
        if (stop) return;
        if (s === SpotifyNotConfigured) {
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
      const url = await spotifyConnect();
      window.location.href = url; // navigate to Spotify authorize (PKCE)
    } catch (e) {
      setError(humanizeThrown(e, "erro ao conectar Spotify"));
      setBusy(false);
    }
  }, []);

  const disconnect = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      await spotifyDisconnect();
      setStatus(EMPTY);
    } catch (e) {
      setError(humanizeThrown(e, "erro ao desconectar Spotify"));
    } finally {
      setBusy(false);
    }
  }, []);

  const search = useCallback(
    async (query: string, kind: SpotifyKind): Promise<void> => {
      if (query.trim() === "") {
        setResults([]);
        return;
      }
      setSearching(true);
      setError(null);
      setImported(null);
      try {
        setResults(await spotifySearch(query, kind));
      } catch (e) {
        setError(humanizeThrown(e, "erro na busca do Spotify"));
        setResults([]);
      } finally {
        setSearching(false);
      }
    },
    [],
  );

  /* busca faixas E playlists de uma vez e mescla (faixas primeiro). allSettled:
     se um tipo falhar (limites de app em Development mode), o outro ainda aparece.
     É o que alimenta a busca automática ao digitar — sem escolher tipo. */
  const searchAll = useCallback(async (query: string): Promise<void> => {
    if (query.trim() === "") {
      setResults([]);
      return;
    }
    setSearching(true);
    setError(null);
    setImported(null);
    try {
      const [tracks, playlists] = await Promise.allSettled([
        spotifySearch(query, "track"),
        spotifySearch(query, "playlist"),
      ]);
      const ok = (r: PromiseSettledResult<CatalogItem[]>): CatalogItem[] =>
        r.status === "fulfilled" ? r.value : [];
      const merged = [...ok(tracks), ...ok(playlists)];
      setResults(merged);
      if (merged.length === 0 && tracks.status === "rejected") {
        setError(humanizeThrown(tracks.reason, "erro na busca do Spotify"));
      }
    } catch (e) {
      setError(humanizeThrown(e, "erro na busca do Spotify"));
      setResults([]);
    } finally {
      setSearching(false);
    }
  }, []);

  /* imports one catalog item: a playlist expands to its tracks, a track imports
     alone. Result count feeds the "importado" feedback. */
  const importItem = useCallback(async (item: CatalogItem): Promise<void> => {
    setBusy(true);
    setError(null);
    setImported(null);
    try {
      const req =
        item.kind === "playlist"
          ? { playlistID: item.uri }
          : { uris: [item.uri] };
      const tracks = await spotifyImport(req);
      setImported(tracks.length);
    } catch (e) {
      setError(humanizeThrown(e, "erro ao importar do Spotify"));
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
  };
}
