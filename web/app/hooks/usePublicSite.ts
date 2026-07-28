"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchPublicSite, HTTPError, type PubSiteDTO } from "@/lib/api";

/* usePublicSite loads the read-only public snapshot (now/upcoming/recent + daily
   info) and refreshes it on an interval so the panel tracks the air. available is
   false once /public/site 404s (option not wired) so the panel shows an empty
   state instead of a stuck spinner. reload() refetches on demand. */
export function usePublicSite(refreshMs = 15000): {
  snapshot: PubSiteDTO | null;
  loaded: boolean;
  busy: boolean;
  available: boolean;
  error: string | null;
  reload: () => Promise<void>;
} {
  const [snapshot, setSnapshot] = useState<PubSiteDTO | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const snap = await fetchPublicSite();
      setSnapshot(snap);
      setAvailable(true);
    } catch (e) {
      if (e instanceof HTTPError && e.status === 404) {
        setAvailable(false);
        setSnapshot(null);
      } else {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void load();
    if (refreshMs <= 0) return;
    const id = setInterval(() => void load(), refreshMs);
    return () => clearInterval(id);
  }, [load, refreshMs]);

  return { snapshot, loaded, busy, available, error, reload: load };
}
