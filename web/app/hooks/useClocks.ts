"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchClocks, saveClock, deleteClock, type ClockDTO } from "@/lib/api";

/* loads the station's clocks/hour templates from GET /api/v1/clocks and exposes
   save()/remove() that PUT/DELETE them, refreshing the list on success. Starts
   with an empty list until the fetch resolves so the modal renders immediately;
   a backend-offline fetch keeps the list empty rather than throwing. */
export function useClocks(): {
  clocks: ClockDTO[];
  loaded: boolean;
  saving: boolean;
  error: string | null;
  save: (c: ClockDTO) => Promise<boolean>;
  remove: (id: string) => Promise<boolean>;
} {
  const [clocks, setClocks] = useState<ClockDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      const data = await fetchClocks();
      setClocks(data);
    } catch {
      /* backend offline → keep the current list */
    }
  }, []);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const data = await fetchClocks();
        if (!stop) setClocks(data);
      } catch {
        /* backend offline → empty list */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  const save = useCallback(
    async (c: ClockDTO): Promise<boolean> => {
      setSaving(true);
      setError(null);
      try {
        await saveClock(c);
        await reload();
        return true;
      } catch (e) {
        setError(e instanceof Error ? e.message : "erro ao salvar");
        return false;
      } finally {
        setSaving(false);
      }
    },
    [reload],
  );

  const remove = useCallback(
    async (id: string): Promise<boolean> => {
      setSaving(true);
      setError(null);
      try {
        await deleteClock(id);
        await reload();
        return true;
      } catch (e) {
        setError(e instanceof Error ? e.message : "erro ao excluir");
        return false;
      } finally {
        setSaving(false);
      }
    },
    [reload],
  );

  return { clocks, loaded, saving, error, save, remove };
}
