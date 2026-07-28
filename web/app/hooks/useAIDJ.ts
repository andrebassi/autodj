"use client";

import { useCallback, useEffect, useState } from "react";
import {
  aidjVoiceover,
  fetchAIDJConfig,
  setAIDJConfig,
  AIDJNotConfigured,
  type AIDJConfig,
  type AIDJVoice,
  type TrackDTO,
} from "@/lib/api";

const EMPTY: AIDJConfig = { enabled: false, voice: "aurora-ptbr" };

/* loads GET /api/v1/aidj/config and exposes toggle()/setVoice()/voiceover().
   `configured` is false when the backend never wired an AIDJService (routes 404),
   so the panel shows "AI DJ não configurado". voiceover(trackId) synthesizes the
   spoken segue for the next track and returns the source=tts mixable Track —
   the voice that bridges a DRM Spotify track on the local bus. A 503 there means
   no TTS gateway is wired (set an ElevenLabs key or Piper model on the server). */
export function useAIDJ(): {
  config: AIDJConfig;
  configured: boolean;
  loaded: boolean;
  busy: boolean;
  error: string | null;
  lastVoiceover: TrackDTO | null;
  toggle: (enabled: boolean) => Promise<void>;
  setVoice: (voice: AIDJVoice) => Promise<void>;
  voiceover: (trackId: string) => Promise<TrackDTO | null>;
} {
  const [config, setConfig] = useState<AIDJConfig>(EMPTY);
  const [configured, setConfigured] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastVoiceover, setLastVoiceover] = useState<TrackDTO | null>(null);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const c = await fetchAIDJConfig();
        if (stop) return;
        if (c === AIDJNotConfigured) {
          setConfigured(false);
          setConfig(EMPTY);
        } else {
          setConfigured(true);
          setConfig(c);
        }
      } catch {
        /* backend offline → keep empty config */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  /* persists a partial config change (enabled or voice) optimistically. */
  const save = useCallback(
    async (next: AIDJConfig): Promise<void> => {
      setBusy(true);
      setError(null);
      const prev = config;
      setConfig(next); // optimistic
      try {
        setConfig(await setAIDJConfig(next));
      } catch (e) {
        setConfig(prev); // rollback
        setError(e instanceof Error ? e.message : "erro ao salvar AI DJ");
      } finally {
        setBusy(false);
      }
    },
    [config],
  );

  const toggle = useCallback(
    (enabled: boolean) => save({ ...config, enabled }),
    [config, save],
  );

  const setVoice = useCallback(
    (voice: AIDJVoice) => save({ ...config, voice }),
    [config, save],
  );

  const voiceover = useCallback(
    async (trackId: string): Promise<TrackDTO | null> => {
      setBusy(true);
      setError(null);
      try {
        const t = await aidjVoiceover(trackId);
        setLastVoiceover(t);
        return t;
      } catch (e) {
        setError(e instanceof Error ? e.message : "erro ao gerar locução");
        return null;
      } finally {
        setBusy(false);
      }
    },
    [],
  );

  return {
    config,
    configured,
    loaded,
    busy,
    error,
    lastVoiceover,
    toggle,
    setVoice,
    voiceover,
  };
}
