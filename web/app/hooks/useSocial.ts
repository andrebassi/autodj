"use client";

import { useCallback, useEffect, useState } from "react";
import {
  announceNowPlaying,
  fetchSocialPlatforms,
  postSocialCustom,
  HTTPError,
  type SocPlatformDTO,
  type SocReceiptDTO,
} from "@/lib/api";

/* useSocial loads the targetable platforms (from the API) and drives the two
   publish actions: announce the current track, or post a free-text note. It
   accumulates the receipts returned so the panel can show what went out.
   available is false once /social/platforms 404s (option not wired). */
export function useSocial(): {
  platforms: SocPlatformDTO[];
  loaded: boolean;
  busy: boolean;
  available: boolean;
  error: string | null;
  receipts: SocReceiptDTO[];
  announce: (keys: string[]) => Promise<void>;
  post: (platform: string, text: string) => Promise<void>;
  reload: () => Promise<void>;
} {
  const [platforms, setPlatforms] = useState<SocPlatformDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [receipts, setReceipts] = useState<SocReceiptDTO[]>([]);

  const load = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const ps = await fetchSocialPlatforms();
      setPlatforms(ps);
      setAvailable(true);
    } catch (e) {
      if (e instanceof HTTPError && e.status === 404) {
        setAvailable(false);
        setPlatforms([]);
      } else {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
      setLoaded(true);
    }
  }, []);

  const mapErr = (e: unknown): string => {
    if (e instanceof HTTPError) {
      if (e.status === 409) return "Nada tocando agora para anunciar.";
      if (e.status === 422) return "Plataforma inválida ou texto vazio.";
      return "Falha ao publicar (" + e.status + ").";
    }
    return e instanceof Error ? e.message : String(e);
  };

  const announce = useCallback(async (keys: string[]): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const out = await announceNowPlaying(keys);
      setReceipts((prev) => [...out.receipts, ...prev]);
    } catch (e) {
      setError(mapErr(e));
    } finally {
      setBusy(false);
    }
  }, []);

  const post = useCallback(async (platform: string, text: string): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const rc = await postSocialCustom(platform, text);
      setReceipts((prev) => [rc, ...prev]);
    } catch (e) {
      setError(mapErr(e));
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  return { platforms, loaded, busy, available, error, receipts, announce, post, reload: load };
}
