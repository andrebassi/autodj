"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchPlayLog,
  fetchTop,
  HTTPError,
  type PlayLogEntryDTO,
  type TopItemDTO,
} from "@/lib/api";

/* rolling window (ms) per period key — mirrors the backend's dia/semana/mes so the
   history panel and the ranking selector agree on the same span. */
const WINDOW_MS: Record<string, number> = {
  dia: 24 * 60 * 60 * 1000,
  semana: 7 * 24 * 60 * 60 * 1000,
  mes: 30 * 24 * 60 * 60 * 1000,
};

/* usePlayLog loads the play history + the "most played" ranking for the selected
   rolling period. History is oldest→newest from the API; the panel shows it newest
   first. reload() refetches both; setPeriod() switches the window and refetches.
   available is false once the /playlog routes 404 (recorder never wired), so the
   panel can show an empty state instead of a stuck spinner. */
export function usePlayLog(initial = "dia"): {
  period: string;
  setPeriod: (p: string) => void;
  history: PlayLogEntryDTO[];
  top: TopItemDTO[];
  loaded: boolean;
  busy: boolean;
  available: boolean;
  error: string | null;
  reload: () => Promise<void>;
} {
  const [period, setPeriodState] = useState(initial);
  const [history, setHistory] = useState<PlayLogEntryDTO[]>([]);
  const [top, setTop] = useState<TopItemDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (p: string): Promise<void> => {
    setBusy(true);
    setError(null);
    const to = new Date();
    const from = new Date(to.getTime() - (WINDOW_MS[p] ?? WINDOW_MS.dia));
    try {
      const [h, t] = await Promise.all([fetchPlayLog(from, to), fetchTop(p, 20)]);
      setHistory([...h].reverse()); // newest first for display
      setTop(t);
      setAvailable(true);
    } catch (e) {
      // a 404 means the recorder isn't wired — show empty, not an error.
      if (e instanceof HTTPError && e.status === 404) {
        setAvailable(false);
        setHistory([]);
        setTop([]);
      } else {
        setError(e instanceof Error ? e.message : "erro ao carregar histórico");
      }
    } finally {
      setBusy(false);
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    let stop = false;
    (async () => {
      if (!stop) await load(period);
    })();
    return () => {
      stop = true;
    };
  }, [period, load]);

  const setPeriod = useCallback((p: string) => setPeriodState(p), []);
  const reload = useCallback(() => load(period), [load, period]);

  return { period, setPeriod, history, top, loaded, busy, available, error, reload };
}
