"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  type CartBoardDTO,
  type CartButtonDTO,
  HTTPError,
  assignCartButton,
  deleteCartButton,
  fetchCartwall,
  trackAudioURL,
} from "@/lib/api";

/* fire mode maps the toolbar toggles to gain behaviour on the local fire bus:
   - fade:    exclusive — fade any playing fires out, then start the new one
   - overlap: sum without fade (polyphonic)
   - talk:    "falar sem baixar" — overlay with no ducking (same local graph as
              overlap; the semantic ducking bus lives server-side in playout) */
export type FireMode = "fade" | "overlap" | "talk";

const FADE_SEC = 0.35;

/* result of an assign: ok, or a typed failure the UI signals (409 dup hotkey /
   422 non-mixable or empty). */
export type AssignResult = { ok: boolean; conflict?: boolean; rejected?: boolean };

export type Cartwall = {
  board: CartBoardDTO;
  pages: number[];
  page: number;
  setPage: (p: number) => void;
  loading: boolean;
  error: string | null;
  mode: FireMode;
  setMode: (m: FireMode) => void;
  loop: boolean;
  toggleLoop: () => void;
  fire: (btn: CartButtonDTO) => void;
  stopAll: () => void;
  louder: () => void;
  softer: () => void;
  volume: number; // 0..1 master gain of the fire bus
  assign: (btn: CartButtonDTO) => Promise<AssignResult>;
  clear: (id: string) => Promise<void>;
  buttonsOn: (page: number) => CartButtonDTO[];
};

/* one active fire: its source + its own gain (so fade-out is per-fire) */
type ActiveFire = { src: AudioBufferSourceNode; gain: GainNode };

/* owns the cartwall board (load/persist) plus a self-contained Web Audio fire
   bus: a master GainNode feeding destination, one ephemeral BufferSource+Gain
   per fire. Reuses the Story 5.1 audio endpoint (decode client-side); the
   backend serves 403 for non-mixable sources so DRM never fires. AudioContext
   is created lazily on the first fire (autoplay policy). A global keydown
   listener fires by hotkey on the current page, ignoring text inputs. */
export function useCartwall(): Cartwall {
  const [board, setBoard] = useState<CartBoardDTO>({ buttons: [] });
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [mode, setMode] = useState<FireMode>("fade");
  const [loop, setLoop] = useState(false);
  const [volume, setVolume] = useState(1);

  const ctxRef = useRef<AudioContext | null>(null);
  const masterRef = useRef<GainNode | null>(null);
  const activeRef = useRef<Set<ActiveFire>>(new Set());
  const bufCache = useRef<Map<string, AudioBuffer>>(new Map());
  const modeRef = useRef<FireMode>("fade");
  const loopRef = useRef(false);
  const volRef = useRef(1);

  modeRef.current = mode;
  loopRef.current = loop;
  volRef.current = volume;

  // initial load — Default() from the backend when the store is empty.
  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const b = await fetchCartwall();
        if (!stop) setBoard(b);
      } catch {
        /* backend offline → empty board, UI still renders */
      } finally {
        if (!stop) setLoading(false);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  const ensureBus = useCallback((): GainNode | null => {
    if (typeof window === "undefined") return null;
    if (!ctxRef.current) {
      const AC = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
      ctxRef.current = new AC();
    }
    const ctx = ctxRef.current;
    if (ctx.state === "suspended") void ctx.resume();
    if (!masterRef.current) {
      const g = ctx.createGain();
      g.gain.value = volRef.current;
      g.connect(ctx.destination);
      masterRef.current = g;
    }
    return masterRef.current;
  }, []);

  const stopFire = useCallback((a: ActiveFire, fade: boolean) => {
    const ctx = ctxRef.current;
    if (!ctx) return;
    if (fade) {
      const now = ctx.currentTime;
      a.gain.gain.setValueAtTime(a.gain.gain.value, now);
      a.gain.gain.linearRampToValueAtTime(0, now + FADE_SEC);
      try {
        a.src.stop(now + FADE_SEC + 0.02);
      } catch {
        /* already stopped */
      }
    } else {
      try {
        a.src.stop();
      } catch {
        /* already stopped */
      }
    }
    activeRef.current.delete(a);
  }, []);

  const stopAll = useCallback(() => {
    for (const a of Array.from(activeRef.current)) stopFire(a, true);
  }, [stopFire]);

  const fire = useCallback(
    (btn: CartButtonDTO) => {
      if (!btn.trackId) return;
      const master = ensureBus();
      const ctx = ctxRef.current;
      if (!master || !ctx) return;
      setError(null);

      // "fade" is exclusive: fade whatever is playing before the new fire.
      if (modeRef.current === "fade") {
        for (const a of Array.from(activeRef.current)) stopFire(a, true);
      }

      const start = (buf: AudioBuffer) => {
        const src = ctx.createBufferSource();
        src.buffer = buf;
        src.loop = loopRef.current;
        const g = ctx.createGain();
        g.gain.value = 1;
        src.connect(g);
        g.connect(master);
        const entry: ActiveFire = { src, gain: g };
        src.onended = () => {
          activeRef.current.delete(entry);
        };
        activeRef.current.add(entry);
        src.start();
      };

      const cached = bufCache.current.get(btn.trackId);
      if (cached) {
        start(cached);
        return;
      }
      (async () => {
        try {
          const res = await fetch(trackAudioURL(btn.trackId));
          if (res.status === 403) throw new Error("faixa não-mixável (Spotify/DRM) não dispara");
          if (!res.ok) throw new Error("áudio: " + res.status);
          const raw = await res.arrayBuffer();
          const buf = await ctx.decodeAudioData(raw);
          bufCache.current.set(btn.trackId, buf);
          start(buf);
        } catch (e) {
          setError(e instanceof Error ? e.message : "erro ao disparar");
        }
      })();
    },
    [ensureBus, stopFire],
  );

  const toggleLoop = useCallback(() => setLoop((v) => !v), []);

  const applyVol = useCallback((v: number) => {
    const clamped = Math.min(1, Math.max(0, v));
    setVolume(clamped);
    const ctx = ctxRef.current;
    if (masterRef.current && ctx) {
      masterRef.current.gain.setValueAtTime(clamped, ctx.currentTime);
    }
  }, []);
  const louder = useCallback(() => applyVol(volRef.current + 0.1), [applyVol]);
  const softer = useCallback(() => applyVol(volRef.current - 0.1), [applyVol]);

  const assign = useCallback(async (btn: CartButtonDTO): Promise<AssignResult> => {
    try {
      const b = await assignCartButton(btn);
      setBoard(b);
      return { ok: true };
    } catch (e) {
      if (e instanceof HTTPError && e.status === 409) {
        setError("hotkey já usada nesta página");
        return { ok: false, conflict: true };
      }
      if (e instanceof HTTPError && e.status === 422) {
        setError("faixa não-mixável ou rótulo vazio");
        return { ok: false, rejected: true };
      }
      setError(e instanceof Error ? e.message : "erro ao salvar botão");
      return { ok: false };
    }
  }, []);

  const clear = useCallback(async (id: string) => {
    try {
      await deleteCartButton(id);
      setBoard((b) => ({ buttons: b.buttons.filter((x) => x.id !== id) }));
    } catch (e) {
      setError(e instanceof Error ? e.message : "erro ao remover botão");
    }
  }, []);

  const pages = useMemo(() => {
    const set = new Set<number>([1]);
    for (const b of board.buttons) if (b.page >= 1) set.add(b.page);
    return Array.from(set).sort((a, z) => a - z);
  }, [board]);

  const buttonsOn = useCallback(
    (p: number) => board.buttons.filter((b) => b.page === p),
    [board],
  );

  // global hotkey: fire the current-page button whose hotkey matches. Ignores
  // typing in inputs/textareas/contenteditable so it never steals keystrokes.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const t = e.target as HTMLElement | null;
      if (t) {
        const tag = t.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || t.isContentEditable) return;
      }
      const key = e.key.toLowerCase();
      const hit = board.buttons.find(
        (b) => b.page === page && b.hotkey && b.hotkey.toLowerCase() === key,
      );
      if (hit) {
        e.preventDefault();
        fire(hit);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [board, page, fire]);

  return {
    board,
    pages,
    page,
    setPage,
    loading,
    error,
    mode,
    setMode,
    loop,
    toggleLoop,
    fire,
    stopAll,
    louder,
    softer,
    volume,
    assign,
    clear,
    buttonsOn,
  };
}
