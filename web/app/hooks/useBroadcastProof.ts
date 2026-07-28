"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchBroadcastProof,
  HTTPError,
  type BroadcastProofDTO,
} from "@/lib/api";

/* rolling window (ms) per period key — matches the play log / ECAD windows so the
   broadcast proof spans the same period as the log it derives from. */
const WINDOW_MS: Record<string, number> = {
  dia: 24 * 60 * 60 * 1000,
  semana: 7 * 24 * 60 * 60 * 1000,
  mes: 30 * 24 * 60 * 60 * 1000,
};

/* useBroadcastProof loads the chronological irradiation proof for the selected
   rolling period. The proof is a read-only projection over the immutable play
   log: only aired plays (broadcasted=true) appear, one line per play, in play
   order — DRM Spotify never shows. available is false once /reports/broadcast
   404s (service not wired). reload()/setPeriod() refetch. */
export function useBroadcastProof(initial = "dia"): {
  period: string;
  setPeriod: (p: string) => void;
  proof: BroadcastProofDTO | null;
  loaded: boolean;
  busy: boolean;
  available: boolean;
  error: string | null;
  reload: () => Promise<void>;
} {
  const [period, setPeriodState] = useState(initial);
  const [proof, setProof] = useState<BroadcastProofDTO | null>(null);
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
      const pr = await fetchBroadcastProof(from, to);
      setProof(pr);
      setAvailable(true);
    } catch (e) {
      // a 404 means the report service isn't wired — show empty, not an error.
      if (e instanceof HTTPError && e.status === 404) {
        setAvailable(false);
        setProof(null);
      } else {
        setError(e instanceof Error ? e.message : "erro ao carregar comprovante");
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

  return { period, setPeriod, proof, loaded, busy, available, error, reload };
}
