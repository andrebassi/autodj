"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchEvents, saveEvent, deleteEvent, type ScheduledEventDTO } from "@/lib/api";

/* loads the station's scheduled events from GET /api/v1/events and exposes
   save()/remove() that PUT/DELETE them, refreshing the list on success. Starts
   with an empty list until the fetch resolves so the modal renders immediately;
   a backend-offline fetch keeps the list empty rather than throwing. */
export function useEvents(): {
  events: ScheduledEventDTO[];
  loaded: boolean;
  saving: boolean;
  error: string | null;
  save: (e: ScheduledEventDTO) => Promise<boolean>;
  remove: (id: string) => Promise<boolean>;
} {
  const [events, setEvents] = useState<ScheduledEventDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      const data = await fetchEvents();
      setEvents(data);
    } catch {
      /* backend offline → keep the current list */
    }
  }, []);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const data = await fetchEvents();
        if (!stop) setEvents(data);
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
    async (e: ScheduledEventDTO): Promise<boolean> => {
      setSaving(true);
      setError(null);
      try {
        await saveEvent(e);
        await reload();
        return true;
      } catch (err) {
        setError(err instanceof Error ? err.message : "erro ao salvar");
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
        await deleteEvent(id);
        await reload();
        return true;
      } catch (err) {
        setError(err instanceof Error ? err.message : "erro ao excluir");
        return false;
      } finally {
        setSaving(false);
      }
    },
    [reload],
  );

  return { events, loaded, saving, error, save, remove };
}
