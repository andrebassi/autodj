"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchUsers,
  createUser,
  updateUser,
  deleteUser,
  HTTPError,
  type UserDTO,
} from "@/lib/api";

/* account management for admins (Story 7.3). Loads GET /users (needs users.manage
   — a 403 sets forbidden), and exposes create/update/remove. Maps the REST status
   codes to pt-BR messages (409 name taken, 422 bad role/name, 404 unknown) so the
   modal can show a precise reason. `active` gates the initial load so the list is
   only fetched while the modal is open and the caller holds the permission. */
export function useUsers(active: boolean): {
  users: UserDTO[];
  loaded: boolean;
  forbidden: boolean;
  busy: boolean;
  error: string | null;
  reload: () => Promise<void>;
  add: (name: string, role: string, password: string) => Promise<boolean>;
  edit: (id: string, role: string, password: string) => Promise<boolean>;
  remove: (id: string) => Promise<boolean>;
} {
  const [users, setUsers] = useState<UserDTO[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [forbidden, setForbidden] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      setUsers(await fetchUsers());
      setForbidden(false);
    } catch (e) {
      if (e instanceof HTTPError && e.status === 403) setForbidden(true);
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    if (!active) return;
    let stop = false;
    (async () => {
      if (!stop) await reload();
    })();
    return () => {
      stop = true;
    };
  }, [active, reload]);

  /* maps a mutation error to a pt-BR reason; returns true when it was handled. */
  const mapErr = useCallback((e: unknown, dupMsg: string): void => {
    if (e instanceof HTTPError) {
      if (e.status === 409) return setError(dupMsg);
      if (e.status === 422) return setError("Papel ou nome inválido");
      if (e.status === 404) return setError("Usuário não encontrado");
      if (e.status === 403) return setError("Sem permissão");
    }
    setError("Falha na operação");
  }, []);

  const run = useCallback(
    async (fn: () => Promise<void>, dupMsg: string): Promise<boolean> => {
      setBusy(true);
      setError(null);
      try {
        await fn();
        await reload();
        return true;
      } catch (e) {
        mapErr(e, dupMsg);
        return false;
      } finally {
        setBusy(false);
      }
    },
    [reload, mapErr],
  );

  const add = useCallback(
    (name: string, role: string, password: string) =>
      run(async () => {
        await createUser({ name, role, password });
      }, "Nome já em uso"),
    [run],
  );
  const edit = useCallback(
    (id: string, role: string, password: string) =>
      run(async () => {
        await updateUser(id, password ? { role, password } : { role });
      }, "Nome já em uso"),
    [run],
  );
  const remove = useCallback(
    (id: string) =>
      run(async () => {
        await deleteUser(id);
      }, ""),
    [run],
  );

  return { users, loaded, forbidden, busy, error, reload, add, edit, remove };
}
