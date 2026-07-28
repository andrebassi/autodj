"use client";

import { useCallback, useEffect, useState } from "react";
import {
  addStaffShift,
  fetchStaffOnAir,
  fetchStaffShifts,
  removeStaffShift,
  HTTPError,
  type StaffOnAirDTO,
  type StaffShiftDTO,
} from "@/lib/api";

/* useStaffSchedule loads the announcer roster (the human grid) from the API and
   drives add/remove plus a poll of who is on air now. available is false once
   /staff/shifts 404s (option not wired in the server build). */
export function useStaffSchedule(): {
  shifts: StaffShiftDTO[];
  onAir: StaffOnAirDTO | null;
  loaded: boolean;
  busy: boolean;
  available: boolean;
  error: string | null;
  add: (input: {
    announcer_id: string;
    announcer_name: string;
    weekday: number;
    start_min: number;
    end_min: number;
  }) => Promise<boolean>;
  remove: (id: string) => Promise<void>;
  reload: () => Promise<void>;
} {
  const [shifts, setShifts] = useState<StaffShiftDTO[]>([]);
  const [onAir, setOnAir] = useState<StaffOnAirDTO | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async (): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const [list, air] = await Promise.all([fetchStaffShifts(), fetchStaffOnAir()]);
      setShifts(list);
      setOnAir(air);
      setAvailable(true);
    } catch (e) {
      if (e instanceof HTTPError && e.status === 404) {
        setAvailable(false);
        setShifts([]);
        setOnAir(null);
      } else {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
      setLoaded(true);
    }
  }, []);

  const add = useCallback(
    async (input: {
      announcer_id: string;
      announcer_name: string;
      weekday: number;
      start_min: number;
      end_min: number;
    }): Promise<boolean> => {
      setBusy(true);
      setError(null);
      try {
        await addStaffShift(input);
        await load();
        return true;
      } catch (e) {
        if (e instanceof HTTPError && e.status === 409) {
          setError("Turno sobrepõe outro locutor no mesmo dia.");
        } else if (e instanceof HTTPError && e.status === 422) {
          setError("Dados do turno inválidos (locutor, dia ou horário).");
        } else {
          setError(e instanceof Error ? e.message : String(e));
        }
        return false;
      } finally {
        setBusy(false);
      }
    },
    [load],
  );

  const remove = useCallback(
    async (id: string): Promise<void> => {
      setBusy(true);
      setError(null);
      try {
        await removeStaffShift(id);
        await load();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setBusy(false);
      }
    },
    [load],
  );

  useEffect(() => {
    void load();
  }, [load]);

  return { shifts, onAir, loaded, busy, available, error, add, remove, reload: load };
}
