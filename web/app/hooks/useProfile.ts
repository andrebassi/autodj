"use client";

import { useCallback, useEffect, useState } from "react";
import {
  fetchProfile,
  saveProfile as apiSaveProfile,
  HTTPError,
  type ProfileDTO,
  type PanelPrefDTO,
} from "@/lib/api";

/* useProfile loads the logged-in operator's player layout (theme/density/panel
   order) once they're authenticated, and persists edits back. Mirrors useAuth:
   the shell passes `enabled=!!user` so the fetch only fires with a live session.
   The backend serves the shipped default when nothing was ever saved, so
   `profile` is never null once ready. save() sends the whole layout (PUT /me,
   owner from token) and adopts the normalized value the server returns. */
export function useProfile(enabled: boolean): {
  profile: ProfileDTO | null;
  ready: boolean;
  busy: boolean;
  error: string | null;
  save: (patch: {
    theme?: string;
    density?: string;
    touch?: boolean;
    screens?: number;
    panels?: PanelPrefDTO[];
  }) => Promise<void>;
} {
  const [profile, setProfile] = useState<ProfileDTO | null>(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) {
      // logged out — drop the layout and re-arm the fetch for the next login.
      setProfile(null);
      setReady(false);
      return;
    }
    let stop = false;
    (async () => {
      try {
        const p = await fetchProfile();
        if (!stop) setProfile(p);
      } catch {
        // no layout yet or transient error — the shell falls back to defaults.
      } finally {
        if (!stop) setReady(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, [enabled]);

  const save = useCallback(
    async (patch: {
      theme?: string;
      density?: string;
      touch?: boolean;
      screens?: number;
      panels?: PanelPrefDTO[];
    }) => {
      // merge the patch over the current layout so a single-field edit (e.g. just
      // theme) still sends a complete, valid body.
      const base = profile;
      if (!base) return;
      const body = {
        theme: patch.theme ?? base.theme,
        density: patch.density ?? base.density,
        touch: patch.touch ?? base.touch,
        screens: patch.screens ?? base.screens,
        panels: patch.panels ?? base.panels,
      };
      setBusy(true);
      setError(null);
      const prev = base;
      setProfile({ ...base, ...body }); // optimistic
      try {
        const out = await apiSaveProfile(body);
        setProfile(out);
      } catch (e) {
        setProfile(prev); // roll back
        setError(
          e instanceof HTTPError && e.status === 422
            ? "Layout inválido"
            : "Falha ao salvar layout",
        );
        throw e;
      } finally {
        setBusy(false);
      }
    },
    [profile],
  );

  return { profile, ready, busy, error, save };
}
