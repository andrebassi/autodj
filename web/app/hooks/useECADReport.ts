"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchECADReport, HTTPError, type ECADReportDTO } from "@/lib/api";

/* rolling window (ms) per period key — mirrors the backend's dia/semana/mes so the
   ECAD report spans the same window as the play log it derives from. */
const WINDOW_MS: Record<string, number> = {
  dia: 24 * 60 * 60 * 1000,
  semana: 7 * 24 * 60 * 60 * 1000,
  mes: 30 * 24 * 60 * 60 * 1000,
};

/* useECADReport loads the ECAD execution report for the selected rolling period.
   The report is a read-only projection over the immutable play log: only
   ECAD-relevant plays appear (DRM Spotify never does), aggregated by track.
   available is false once /reports/ecad 404s (service not wired), so the panel
   shows an empty state instead of a stuck spinner. reload()/setPeriod() refetch. */
export function useECADReport(initial = "dia"): {
  period: string;
  setPeriod: (p: string) => void;
  report: ECADReportDTO | null;
  loaded: boolean;
  busy: boolean;
  available: boolean;
  error: string | null;
  reload: () => Promise<void>;
} {
  const [period, setPeriodState] = useState(initial);
  const [report, setReport] = useState<ECADReportDTO | null>(null);
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
      const rep = await fetchECADReport(from, to);
      setReport(rep);
      setAvailable(true);
    } catch (e) {
      // a 404 means the report service isn't wired — show empty, not an error.
      if (e instanceof HTTPError && e.status === 404) {
        setAvailable(false);
        setReport(null);
      } else {
        setError(e instanceof Error ? e.message : "erro ao carregar relatório");
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

  return { period, setPeriod, report, loaded, busy, available, error, reload };
}
