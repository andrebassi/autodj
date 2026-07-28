"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { trackAudioURL } from "@/lib/api";
import { pitchToRate, rateToBPM } from "@/lib/tempo";

export type DeckState = "empty" | "loading" | "loaded" | "playing" | "paused" | "ended";

export const SLOT_COUNT = 8;

export type EQBand = "low" | "mid" | "high";
export type EQState = { low: number; mid: number; high: number }; // dB
export const EQ_KILL = -40; // dB — effective band kill

/* one Web Audio deck. Owns a master GainNode (fed by the crossfader) plus an
   independent cue path (a MediaStreamAudioDestinationNode + <audio>.setSinkId)
   so the operator can pre-listen on a separate output regardless of the master
   fade. Decodes a track into an AudioBuffer and drives play/pause/stop/seek via
   one-shot AudioBufferSourceNodes (Web Audio sources can't restart). position/
   duration derive from the context clock, not state, so the playhead is smooth.
   Refuses non-mixable tracks before fetching (two-engine invariant). */
export type Deck = {
  state: DeckState;
  trackId: string | null;
  title: string;
  duration: number; // seconds
  position: number; // seconds (live)
  error: string | null;
  gain: GainNode | null;
  hotCues: number[]; // 8 slots, ms or -1 (empty)
  cueOn: boolean;
  bpm: number; // analysed BPM of the loaded track (0 = unknown)
  pitchPct: number; // pitch fader position, ±8
  effectiveBPM: number; // bpm at the current playbackRate
  eq: EQState;
  load: (t: {
    id: string;
    title: string;
    mixable: boolean;
    bpm?: number;
    markers?: { pos_ms: number }[];
  }) => Promise<void>;
  play: () => void;
  pause: () => void;
  stop: () => void;
  eject: () => void;
  seek: (sec: number) => void;
  setCue: (slot: number) => void; // store current position at slot
  jumpCue: (slot: number) => void; // seek to slot
  clearCue: (slot: number) => void;
  toggleCue: () => void; // pre-listen on/off
  setCueDevice: (deviceId: string) => void;
  setEQ: (band: EQBand, db: number) => void;
  setPitch: (pct: number) => void; // ±8; clamp is domain's job
};

const EMPTY_CUES = () => new Array<number>(SLOT_COUNT).fill(-1);

export function useDeck(ctxRef: React.MutableRefObject<AudioContext | null>): Deck {
  const [state, setState] = useState<DeckState>("empty");
  const [trackId, setTrackId] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [duration, setDuration] = useState(0);
  const [position, setPosition] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [hotCues, setHotCues] = useState<number[]>(EMPTY_CUES);
  const [cueOn, setCueOn] = useState(false);
  const [bpm, setBpm] = useState(0);
  const [pitchPct, setPitchPct] = useState(0);
  const [eq, setEq] = useState<EQState>({ low: 0, mid: 0, high: 0 });

  const gainRef = useRef<GainNode | null>(null);
  const cueGainRef = useRef<GainNode | null>(null);
  const cueDestRef = useRef<MediaStreamAudioDestinationNode | null>(null);
  const cueAudioRef = useRef<HTMLAudioElement | null>(null);
  const eqLowRef = useRef<BiquadFilterNode | null>(null);
  const eqMidRef = useRef<BiquadFilterNode | null>(null);
  const eqHighRef = useRef<BiquadFilterNode | null>(null);
  const bufRef = useRef<AudioBuffer | null>(null);
  const srcRef = useRef<AudioBufferSourceNode | null>(null);
  const startedAtRef = useRef(0);
  const offsetRef = useRef(0);
  const pitchRef = useRef(0);
  const rateAtStartRef = useRef(1); // playbackRate captured when the source started
  const [gain, setGain] = useState<GainNode | null>(null);

  // master gain + cue path + 3-band EQ (lazy, once the context exists).
  // graph: source → eqLow(lowshelf) → eqMid(peaking) → eqHigh(highshelf) →
  //   { gain (master, faded by crossfader), cueGain (pre-listen) }.
  // ensureNodes returns the EQ input (eqLow) — where a fresh source connects.
  const ensureNodes = useCallback((): BiquadFilterNode | null => {
    const ctx = ctxRef.current;
    if (!ctx) return null;
    if (!gainRef.current) {
      const g = ctx.createGain();
      g.connect(ctx.destination);
      gainRef.current = g;
      setGain(g);
    }
    if (!cueGainRef.current) {
      const cg = ctx.createGain();
      cg.gain.value = 0; // cue off until toggled
      const dest = ctx.createMediaStreamDestination();
      cg.connect(dest);
      cueGainRef.current = cg;
      cueDestRef.current = dest;
      const el = new Audio();
      el.srcObject = dest.stream;
      void el.play().catch(() => {});
      cueAudioRef.current = el;
    }
    if (!eqLowRef.current) {
      const low = ctx.createBiquadFilter();
      low.type = "lowshelf";
      low.frequency.value = 250;
      const mid = ctx.createBiquadFilter();
      mid.type = "peaking";
      mid.frequency.value = 1500;
      mid.Q.value = 0.8;
      const high = ctx.createBiquadFilter();
      high.type = "highshelf";
      high.frequency.value = 4000;
      low.connect(mid);
      mid.connect(high);
      high.connect(gainRef.current);
      if (cueGainRef.current) high.connect(cueGainRef.current);
      eqLowRef.current = low;
      eqMidRef.current = mid;
      eqHighRef.current = high;
    }
    return eqLowRef.current;
  }, [ctxRef]);

  const stopSource = useCallback(() => {
    if (srcRef.current) {
      srcRef.current.onended = null;
      try {
        srcRef.current.stop();
      } catch {
        /* already stopped */
      }
      srcRef.current.disconnect();
      srcRef.current = null;
    }
  }, []);

  // start a fresh one-shot source from the current offset (shared by play/seek)
  const startFrom = useCallback(
    (offsetSec: number) => {
      const ctx = ctxRef.current;
      const buf = bufRef.current;
      const eqIn = ensureNodes();
      if (!ctx || !buf || !eqIn) return;
      if (ctx.state === "suspended") void ctx.resume();
      stopSource();
      const src = ctx.createBufferSource();
      src.buffer = buf;
      const rate = pitchToRate(pitchRef.current);
      src.playbackRate.value = rate;
      rateAtStartRef.current = rate;
      src.connect(eqIn);
      const off = ((offsetSec % buf.duration) + buf.duration) % buf.duration;
      src.start(0, off);
      startedAtRef.current = ctx.currentTime;
      offsetRef.current = off;
      src.onended = () => {
        offsetRef.current = 0;
        setPosition(0);
        setState("ended");
      };
      srcRef.current = src;
      setState("playing");
    },
    [ctxRef, ensureNodes, stopSource],
  );

  const load = useCallback(
    async (t: {
      id: string;
      title: string;
      mixable: boolean;
      bpm?: number;
      markers?: { pos_ms: number }[];
    }): Promise<void> => {
      if (!t.mixable) {
        setError("faixa não-mixável (Spotify/DRM) não carrega no deck");
        return;
      }
      const ctx = ctxRef.current;
      if (!ctx) return;
      ensureNodes();
      stopSource();
      setState("loading");
      setError(null);
      setPosition(0);
      offsetRef.current = 0;
      setBpm(t.bpm && t.bpm > 0 ? t.bpm : 0);
      setHotCues(EMPTY_CUES());
      try {
        const res = await fetch(trackAudioURL(t.id));
        if (!res.ok) throw new Error("audio: " + res.status);
        const raw = await res.arrayBuffer();
        const buf = await ctx.decodeAudioData(raw);
        bufRef.current = buf;
        setTrackId(t.id);
        setTitle(t.title);
        setDuration(buf.duration);
        // seed first slots from cue markers if present
        if (t.markers?.length) {
          const cues = EMPTY_CUES();
          const sorted = [...t.markers].sort((a, b) => a.pos_ms - b.pos_ms);
          sorted.slice(0, SLOT_COUNT).forEach((m, i) => {
            cues[i] = Math.min(Math.max(0, m.pos_ms), buf.duration * 1000);
          });
          setHotCues(cues);
        }
        setState("loaded");
      } catch (e) {
        bufRef.current = null;
        setState("empty");
        setError(e instanceof Error ? e.message : "falha ao carregar áudio");
      }
    },
    [ctxRef, ensureNodes, stopSource],
  );

  const play = useCallback(() => {
    if (!bufRef.current) return;
    startFrom(offsetRef.current);
  }, [startFrom]);

  const pause = useCallback(() => {
    const ctx = ctxRef.current;
    if (!ctx || state !== "playing") return;
    offsetRef.current += (ctx.currentTime - startedAtRef.current) * rateAtStartRef.current;
    stopSource();
    setPosition(offsetRef.current);
    setState("paused");
  }, [ctxRef, state, stopSource]);

  const stop = useCallback(() => {
    stopSource();
    offsetRef.current = 0;
    setPosition(0);
    if (bufRef.current) setState("loaded");
  }, [stopSource]);

  const eject = useCallback(() => {
    stopSource();
    bufRef.current = null;
    offsetRef.current = 0;
    setTrackId(null);
    setTitle("");
    setDuration(0);
    setPosition(0);
    setError(null);
    setHotCues(EMPTY_CUES());
    setBpm(0);
    pitchRef.current = 0;
    rateAtStartRef.current = 1;
    setPitchPct(0);
    setState("empty");
  }, [stopSource]);

  const seek = useCallback(
    (sec: number) => {
      if (!bufRef.current) return;
      const clamped = Math.min(Math.max(0, sec), bufRef.current.duration);
      if (state === "playing") {
        startFrom(clamped); // restart at new offset, keep playing
      } else {
        offsetRef.current = clamped;
        setPosition(clamped);
      }
    },
    [state, startFrom],
  );

  const setCue = useCallback(
    (slot: number) => {
      if (slot < 0 || slot >= SLOT_COUNT) return;
      const ctx = ctxRef.current;
      let posSec = offsetRef.current;
      if (ctx && state === "playing")
        posSec += (ctx.currentTime - startedAtRef.current) * rateAtStartRef.current;
      setHotCues((c) => {
        const next = [...c];
        next[slot] = Math.round(posSec * 1000);
        return next;
      });
    },
    [ctxRef, state],
  );

  const jumpCue = useCallback(
    (slot: number) => {
      if (slot < 0 || slot >= SLOT_COUNT) return;
      const ms = hotCues[slot];
      if (ms < 0) return;
      seek(ms / 1000);
    },
    [hotCues, seek],
  );

  const clearCue = useCallback((slot: number) => {
    if (slot < 0 || slot >= SLOT_COUNT) return;
    setHotCues((c) => {
      const next = [...c];
      next[slot] = -1;
      return next;
    });
  }, []);

  const toggleCue = useCallback(() => {
    ensureNodes();
    setCueOn((on) => {
      const next = !on;
      const cg = cueGainRef.current;
      const ctx = ctxRef.current;
      if (cg) cg.gain.setValueAtTime(next ? 1 : 0, ctx ? ctx.currentTime : 0);
      return next;
    });
  }, [ensureNodes, ctxRef]);

  const setCueDevice = useCallback((deviceId: string) => {
    const el = cueAudioRef.current as (HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> }) | null;
    if (el?.setSinkId) void el.setSinkId(deviceId).catch(() => {});
  }, []);

  const setEQ = useCallback((band: EQBand, db: number) => {
    const ctx = ctxRef.current;
    const node =
      band === "low" ? eqLowRef.current : band === "mid" ? eqMidRef.current : eqHighRef.current;
    if (node) node.gain.setValueAtTime(db, ctx ? ctx.currentTime : 0);
    setEq((e) => ({ ...e, [band]: db }));
  }, [ctxRef]);

  // pitch fader: domain clamps ±8. Re-anchor while playing so the playhead stays
  // continuous when the rate changes mid-flight.
  const setPitch = useCallback(
    (pct: number) => {
      const clamped = pct > 8 ? 8 : pct < -8 ? -8 : pct;
      const ctx = ctxRef.current;
      if (ctx && state === "playing" && srcRef.current) {
        offsetRef.current += (ctx.currentTime - startedAtRef.current) * rateAtStartRef.current;
        startedAtRef.current = ctx.currentTime;
        const rate = pitchToRate(clamped);
        rateAtStartRef.current = rate;
        srcRef.current.playbackRate.setValueAtTime(rate, ctx.currentTime);
      }
      pitchRef.current = clamped;
      setPitchPct(clamped);
    },
    [ctxRef, state],
  );

  // live position readout while playing
  useEffect(() => {
    if (state !== "playing") return;
    let raf = 0;
    const tick = () => {
      const ctx = ctxRef.current;
      const buf = bufRef.current;
      if (ctx && buf) {
        const p = offsetRef.current + (ctx.currentTime - startedAtRef.current) * rateAtStartRef.current;
        setPosition(Math.min(p, buf.duration));
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [state, ctxRef]);

  return {
    state,
    trackId,
    title,
    duration,
    position,
    error,
    gain,
    hotCues,
    cueOn,
    bpm,
    pitchPct,
    effectiveBPM: bpm > 0 ? rateToBPM(bpm, pitchToRate(pitchPct)) : 0,
    eq,
    load,
    play,
    pause,
    stop,
    eject,
    seek,
    setCue,
    jumpCue,
    clearCue,
    toggleCue,
    setCueDevice,
    setEQ,
    setPitch,
  };
}
