"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchLineIn,
  armLineIn,
  disarmLineIn,
  setLineInLevels,
  LineInNotConfigured,
  type LineInInput,
} from "@/lib/api";

/* loads GET /api/v1/linein and exposes arm()/disarm()/setLevels().
   `configured` is false when the backend never wired a LineInService (routes
   404), so the panel shows "Line-in não configurado". Line-in is the legitimate
   *live* PCM source — always mixable, never DRM — so arming it ducks the music
   without breaching the two-engine invariant (Spotify stays unmixable). Updates
   are optimistic per-input, rolling back on error. */
export function useLineIn(): {
  inputs: LineInInput[];
  configured: boolean;
  loaded: boolean;
  busy: string | null;
  error: string | null;
  arm: (id: string) => Promise<void>;
  disarm: (id: string) => Promise<void>;
  setGain: (id: string, gain: number) => Promise<void>;
  setDuck: (id: string, duck: number) => Promise<void>;
} {
  const [inputs, setInputs] = useState<LineInInput[]>([]);
  const [configured, setConfigured] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const list = await fetchLineIn();
        if (stop) return;
        if (list === LineInNotConfigured) {
          setConfigured(false);
          setInputs([]);
        } else {
          setConfigured(true);
          setInputs(list);
        }
      } catch {
        /* backend offline → keep empty */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  /* replaces one input in the list by id. */
  const merge = useCallback((next: LineInInput) => {
    setInputs((cur) => cur.map((i) => (i.id === next.id ? next : i)));
  }, []);

  /* runs a mutating call for id, tracking busy/error and merging the result. */
  const run = useCallback(
    async (id: string, fn: () => Promise<LineInInput>, msg: string) => {
      setBusy(id);
      setError(null);
      const prev = inputs.find((i) => i.id === id);
      try {
        merge(await fn());
      } catch (e) {
        if (prev) merge(prev); // rollback
        setError(e instanceof Error ? e.message : msg);
      } finally {
        setBusy(null);
      }
    },
    [inputs, merge],
  );

  const arm = useCallback(
    (id: string) => run(id, () => armLineIn(id), "erro ao armar line-in"),
    [run],
  );
  const disarm = useCallback(
    (id: string) => run(id, () => disarmLineIn(id), "erro ao desarmar line-in"),
    [run],
  );
  const setGain = useCallback(
    (id: string, gain: number) =>
      run(id, () => setLineInLevels(id, { gain }), "erro ao ajustar ganho"),
    [run],
  );
  const setDuck = useCallback(
    (id: string, duck: number) =>
      run(id, () => setLineInLevels(id, { duck }), "erro ao ajustar duck"),
    [run],
  );

  return {
    inputs,
    configured,
    loaded,
    busy,
    error,
    arm,
    disarm,
    setGain,
    setDuck,
  };
}
