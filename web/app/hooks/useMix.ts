"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchMix, saveMix } from "@/lib/api";
import { EMPTY_OPERATIONAL, type MixConfig } from "@/lib/uiContent";

/* loads the persisted fine-mixing settings from GET /api/v1/mix and exposes a
   save() that PUTs them. Returns EMPTY_OPERATIONAL.mix (the seed default, kept
   in parity with the backend) until the fetch resolves so the modal renders
   immediately. `seed` (e.g. op.mix from the operational catalog) overrides the
   initial value while the network call is in flight. */
export function useMix(seed?: MixConfig): {
  mix: MixConfig;
  loaded: boolean;
  saving: boolean;
  error: string | null;
  save: (m: MixConfig) => Promise<boolean>;
} {
  const [mix, setMix] = useState<MixConfig>(seed ?? EMPTY_OPERATIONAL.mix);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const data = await fetchMix();
        if (!stop && data) setMix(data);
      } catch {
        /* backend offline → keep the seed/default */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  const save = useCallback(async (m: MixConfig): Promise<boolean> => {
    setSaving(true);
    setError(null);
    try {
      const stored = await saveMix(m);
      setMix(stored);
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : "erro ao salvar");
      return false;
    } finally {
      setSaving(false);
    }
  }, []);

  return { mix, loaded, saving, error, save };
}
