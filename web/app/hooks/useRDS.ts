"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchRDSConfig,
  setRDSConfig,
  fetchRDSState,
  refreshRDS,
  RDSNotConfigured,
  type RDSConfig,
  type RDSState,
} from "@/lib/api";

const EMPTY: RDSConfig = { enabled: false, pi: 0x1001, ps: "AUTODJ", pty: 10 };

/* loads GET /api/v1/rds/config and exposes toggle()/save()/refresh().
   `configured` is false when the backend never wired an RDSService (routes 404),
   so the panel shows "RDS não configurado". RDS carries text-only metadata
   (PI/PS/PTY/RadioText) — no audio — so it never touches the two-engine invariant.
   refresh(title,artist) rebuilds and pushes the state to the encoder; a 503 there
   means no encoder is wired (set AUTODJ_RDS_FILE on the server). */
export function useRDS(): {
  config: RDSConfig;
  configured: boolean;
  loaded: boolean;
  busy: boolean;
  error: string | null;
  state: RDSState | null;
  toggle: (enabled: boolean) => Promise<void>;
  save: (cfg: RDSConfig) => Promise<void>;
  refresh: (title: string, artist: string) => Promise<RDSState | null>;
} {
  const [config, setConfig] = useState<RDSConfig>(EMPTY);
  const [configured, setConfigured] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [state, setState] = useState<RDSState | null>(null);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const c = await fetchRDSConfig();
        if (stop) return;
        if (c === RDSNotConfigured) {
          setConfigured(false);
          setConfig(EMPTY);
        } else {
          setConfigured(true);
          setConfig(c);
          try {
            setState(await fetchRDSState());
          } catch {
            /* state optional */
          }
        }
      } catch {
        /* backend offline → keep empty config */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  /* persists the identity optimistically, rolling back on error. */
  const save = useCallback(
    async (next: RDSConfig): Promise<void> => {
      setBusy(true);
      setError(null);
      const prev = config;
      setConfig(next); // optimistic
      try {
        setConfig(await setRDSConfig(next));
      } catch (e) {
        setConfig(prev); // rollback
        setError(e instanceof Error ? e.message : "erro ao salvar RDS");
      } finally {
        setBusy(false);
      }
    },
    [config],
  );

  const toggle = useCallback(
    (enabled: boolean) => save({ ...config, enabled }),
    [config, save],
  );

  const refresh = useCallback(
    async (title: string, artist: string): Promise<RDSState | null> => {
      setBusy(true);
      setError(null);
      try {
        const s = await refreshRDS(title, artist);
        setState(s);
        return s;
      } catch (e) {
        setError(e instanceof Error ? e.message : "erro ao atualizar RDS");
        return null;
      } finally {
        setBusy(false);
      }
    },
    [],
  );

  return {
    config,
    configured,
    loaded,
    busy,
    error,
    state,
    toggle,
    save,
    refresh,
  };
}
