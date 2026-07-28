"use client";

import { useState } from "react";
import type { CartButtonDTO } from "@/lib/api";
import type { AssignResult } from "@/hooks/useCartwall";

/* one mixable track offered in the edit picker */
export type TrackOption = { id: string; label: string };

/* one cartwall pad. In fire mode a click disparaes the button's track through
   the Web Audio bus; the hotkey shows as a corner badge. In edit mode it flips
   to an inline form (label / hotkey / track) that upserts via onAssign — a 409
   (hotkey clash) is flagged red — plus a remove (✕). One component, one file. */
export function CartButton({
  btn,
  editing,
  trackOptions,
  onFire,
  onAssign,
  onClear,
}: {
  btn: CartButtonDTO;
  editing: boolean;
  trackOptions: TrackOption[];
  onFire: (btn: CartButtonDTO) => void;
  onAssign: (btn: CartButtonDTO) => Promise<AssignResult>;
  onClear: (id: string) => void;
}) {
  const [label, setLabel] = useState(btn.label);
  const [hotkey, setHotkey] = useState(btn.hotkey);
  const [trackId, setTrackId] = useState(btn.trackId);
  const [conflict, setConflict] = useState(false);

  const bg = `linear-gradient(160deg, hsl(${btn.hue} 55% 92%), hsl(${btn.hue} 45% 82%))`;

  if (editing) {
    const save = async () => {
      setConflict(false);
      const res = await onAssign({ ...btn, label, hotkey, trackId });
      if (res.conflict) setConflict(true);
    };
    return (
      <div className={"cw-btn cw-edit" + (conflict ? " cw-clash" : "")} style={{ background: bg }}>
        <input
          className="cw-ein"
          value={label}
          placeholder="Rótulo"
          onChange={(e) => setLabel(e.target.value)}
          onBlur={save}
        />
        <select
          className="cw-esel"
          value={trackId}
          onChange={(e) => setTrackId(e.target.value)}
          onBlur={save}
        >
          <option value="">— faixa —</option>
          {trackOptions.map((t) => (
            <option key={t.id} value={t.id}>
              {t.label}
            </option>
          ))}
        </select>
        <div className="cw-erow">
          <input
            className="cw-ekey"
            value={hotkey}
            maxLength={1}
            placeholder="tecla"
            onChange={(e) => setHotkey(e.target.value)}
            onBlur={save}
          />
          <button className="cw-edel" title="Remover" onClick={() => onClear(btn.id)}>
            ✕
          </button>
        </div>
      </div>
    );
  }

  return (
    <button className="cw-btn" style={{ background: bg }} onClick={() => onFire(btn)}>
      {btn.hotkey && <span className="cw-hk">{btn.hotkey.toUpperCase()}</span>}
      <span className="cw-ico">{btn.icon || "▶"}</span>
      <span className="cw-lbl">{btn.label}</span>
    </button>
  );
}
