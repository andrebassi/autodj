"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchGrid, saveGrid, type GridDTO } from "@/lib/api";

/* loads the weekly programming grid from GET /api/v1/grid and exposes save()
   that PUTs the whole grid, refreshing on success. Starts empty until the fetch
   resolves so the modal renders immediately; a backend-offline fetch keeps the
   grid empty rather than throwing. */
export function useGrid(): {
  grid: GridDTO;
  loaded: boolean;
  saving: boolean;
  error: string | null;
  save: (g: GridDTO) => Promise<boolean>;
} {
  const [grid, setGrid] = useState<GridDTO>({ cells: [] });
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const data = await fetchGrid();
        if (!stop) setGrid(data);
      } catch {
        /* backend offline → empty grid */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  const save = useCallback(async (g: GridDTO): Promise<boolean> => {
    setSaving(true);
    setError(null);
    try {
      const stored = await saveGrid(g);
      setGrid(stored);
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "erro ao salvar grade");
      return false;
    } finally {
      setSaving(false);
    }
  }, []);

  return { grid, loaded, saving, error, save };
}
