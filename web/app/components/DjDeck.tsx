"use client";

import type { Deck } from "@/hooks/useDeck";
import type { OpMedia } from "@/lib/operational";
import { HotCuePad } from "@/components/HotCuePad";
import { CueDeviceSelect } from "@/components/CueDeviceSelect";
import { EQ3Knobs } from "@/components/EQ3Knobs";
import { PitchFader } from "@/components/PitchFader";
import { sync } from "@/lib/tempo";

/* mm:ss from seconds */
function clock(sec: number): string {
  const s = Math.max(0, Math.floor(sec));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

/* one interactive Web Audio deck (A or B). Picker lists only mixable/PCM tracks;
   the live playhead, times and enabled/disabled transport come from the hook.
   Loading a non-mixable track is impossible (picker excludes them) and the hook
   also refuses it — the two-engine invariant holds on the client too. */
export function DjDeck({
  deck,
  d,
  tracks,
  ready,
  onArm,
  masterBPM,
}: {
  deck: "A" | "B";
  d: Deck;
  tracks: OpMedia[]; // pre-filtered: mixable && local
  ready: boolean;
  onArm: () => void;
  masterBPM: number; // other deck's effective BPM (0 = unknown)
}) {
  const frac = d.duration > 0 ? d.position / d.duration : 0;
  const loaded = d.state !== "empty" && d.state !== "loading";
  const playing = d.state === "playing";

  const pick = (id: string) => {
    onArm(); // first gesture unlocks the AudioContext
    const t = tracks.find((x) => x.id === id);
    if (t?.id)
      void d.load({ id: t.id, title: `${t.artist} — ${t.title}`, mixable: true, bpm: t.bpm });
  };

  const doSync = () => {
    const r = sync(d.bpm, masterBPM); // beat-match this deck to the master
    if (r) d.setPitch(r.pitchPct);
  };

  return (
    <div className={"djdeck djdeck-" + deck.toLowerCase() + (playing ? " live" : "")}>
      <div className="djdeck-top">
        <span className="djdeck-badge">{deck}</span>
        <select
          className="djdeck-pick"
          value={d.trackId ?? ""}
          onChange={(e) => pick(e.target.value)}
          aria-label={"Carregar faixa no deck " + deck}
        >
          <option value="">— carregar faixa —</option>
          {tracks.map((t) => (
            <option key={t.id} value={t.id}>
              {t.artist} — {t.title}
            </option>
          ))}
        </select>
      </div>

      <div className="djdeck-title">
        {d.title || <span className="djdeck-empty">deck vazio</span>}
        {d.state === "loading" && <span className="djdeck-tag">carregando…</span>}
      </div>

      <div
        className="djdeck-wave"
        role="slider"
        aria-label={"Playhead deck " + deck}
        aria-valuenow={Math.round(frac * 100)}
        onClick={(e) => {
          if (!loaded || d.duration <= 0) return;
          const r = e.currentTarget.getBoundingClientRect();
          const p = Math.min(Math.max(0, (e.clientX - r.left) / r.width), 1);
          d.seek(p * d.duration);
        }}
      >
        <span className="djdeck-fill" style={{ width: `${frac * 100}%` }} />
        <span className="djdeck-head" style={{ left: `${frac * 100}%` }} />
      </div>

      <div className="djdeck-times">
        <span>{clock(d.position)}</span>
        <span>-{clock(Math.max(0, d.duration - d.position))}</span>
      </div>

      <div className="djdeck-transport">
        <button disabled={!ready || !loaded} onClick={() => (playing ? d.pause() : d.play())}>
          {playing ? "❚❚" : "▶"}
        </button>
        <button disabled={!loaded} onClick={() => d.stop()} title="Cue (volta ao início)">
          ⏮
        </button>
        <button disabled={!loaded} onClick={() => d.eject()} title="Ejetar">
          ⏏
        </button>
      </div>

      <div className="djdeck-mix">
        <EQ3Knobs d={d} disabled={!loaded} />
        <PitchFader d={d} masterBPM={masterBPM} onSync={doSync} />
      </div>

      <HotCuePad d={d} disabled={!loaded} />
      <CueDeviceSelect d={d} disabled={!ready} />

      {d.error && <div className="djdeck-err">{d.error}</div>}
    </div>
  );
}
