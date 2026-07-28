"use client";

import { useEffect, useState } from "react";
import { fetchWaveform } from "@/lib/api";

/* fetches the server-side waveform peaks for a PCM track. Non-PCM tracks (id
   null/undefined or pcm=false) skip the fetch and return empty peaks — the
   editor renders the DRM notice instead. Peaks are normalized [0,1]. */
export function useWaveform(id: string | undefined, pcm: boolean, bars = 160) {
  const [peaks, setPeaks] = useState<number[]>([]);
  const [durationMs, setDurationMs] = useState(0);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!id || !pcm) {
      setPeaks([]);
      setDurationMs(0);
      return;
    }
    let stop = false;
    setLoading(true);
    (async () => {
      try {
        const wf = await fetchWaveform(id, bars);
        if (!stop) {
          setPeaks(wf.peaks ?? []);
          setDurationMs(wf.duration_ms);
        }
      } catch {
        if (!stop) setPeaks([]);
      } finally {
        if (!stop) setLoading(false);
      }
    })();
    return () => {
      stop = true;
    };
  }, [id, pcm, bars]);

  return { peaks, durationMs, loading };
}
