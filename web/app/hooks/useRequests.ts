"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchRequests,
  moderateRequest,
  submitRequest,
  HTTPError,
  type RequestDTO,
} from "@/lib/api";

/* useRequests drives the listener request / dedication moderation queue (Story
   8.3). It loads the queue for the selected status filter, submits new requests,
   and applies moderation transitions (approve/reject/played). available is false
   once /requests 404s (service never wired), so the panel shows an empty state
   instead of a stuck spinner. reload() refetches; setFilter() switches the status
   view and refetches. */
export function useRequests(initial = ""): {
  filter: string;
  setFilter: (s: string) => void;
  items: RequestDTO[];
  loaded: boolean;
  busy: boolean;
  available: boolean;
  error: string | null;
  reload: () => Promise<void>;
  submit: (listener: string, songHint: string, dedication: string) => Promise<boolean>;
  moderate: (id: string, action: "approve" | "reject" | "played") => Promise<void>;
} {
  const [filter, setFilterState] = useState(initial);
  const [items, setItems] = useState<RequestDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (status: string): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const list = await fetchRequests(status);
      setItems(list);
      setAvailable(true);
    } catch (e) {
      if (e instanceof HTTPError && e.status === 404) {
        setAvailable(false);
        setItems([]);
      } else {
        setError(e instanceof Error ? e.message : "erro ao carregar pedidos");
      }
    } finally {
      setBusy(false);
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    let stop = false;
    (async () => {
      if (!stop) await load(filter);
    })();
    return () => {
      stop = true;
    };
  }, [filter, load]);

  const setFilter = useCallback((s: string) => setFilterState(s), []);
  const reload = useCallback(() => load(filter), [load, filter]);

  const submit = useCallback(
    async (listener: string, songHint: string, dedication: string): Promise<boolean> => {
      setError(null);
      try {
        await submitRequest(listener, songHint, dedication);
        await load(filter);
        return true;
      } catch (e) {
        if (e instanceof HTTPError && e.status === 422) {
          setError("informe o nome e a música do pedido");
        } else {
          setError(e instanceof Error ? e.message : "erro ao enviar pedido");
        }
        return false;
      }
    },
    [load, filter],
  );

  const moderate = useCallback(
    async (id: string, action: "approve" | "reject" | "played"): Promise<void> => {
      setError(null);
      try {
        await moderateRequest(id, action);
        await load(filter);
      } catch (e) {
        if (e instanceof HTTPError && e.status === 409) {
          setError("transição não permitida para esse pedido");
        } else {
          setError(e instanceof Error ? e.message : "erro ao moderar pedido");
        }
      }
    },
    [load, filter],
  );

  return { filter, setFilter, items, loaded, busy, available, error, reload, submit, moderate };
}
