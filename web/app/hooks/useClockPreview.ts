"use client";

import { useCallback, useState } from "react";
import { generatePlaylist, type TrackDTO } from "@/lib/api";

/* generates a read-only preview playlist for a clock via POST
   /clocks/{id}/generate. No track is played. run(id) fills tracks on success;
   a 404 (unknown clock) or 422 (empty catalog) surfaces as an error string.
   clear() resets so the modal can close the preview. */
export function useClockPreview(): {
  tracks: TrackDTO[];
  loading: boolean;
  error: string | null;
  run: (clockId: string) => Promise<void>;
  clear: () => void;
} {
  const [tracks, setTracks] = useState<TrackDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(async (clockId: string) => {
    setLoading(true);
    setError(null);
    try {
      const list = await generatePlaylist(clockId);
      setTracks(list);
      if (list.length === 0) setError("catálogo vazio para este relógio");
    } catch (e) {
      const msg = e instanceof Error ? e.message : "erro ao prever";
      setError(
        msg.endsWith("422")
          ? "catálogo sem faixas elegíveis (só Spotify/DRM não conta)"
          : msg.endsWith("404")
            ? "relógio desconhecido"
            : "erro ao prever hora",
      );
      setTracks([]);
    } finally {
      setLoading(false);
    }
  }, []);

  const clear = useCallback(() => {
    setTracks([]);
    setError(null);
  }, []);

  return { tracks, loading, error, run, clear };
}
