"use client";

import type { FireMode } from "@/hooks/useCartwall";

/* cartwall footer: the 3 fire-mode radios (Fade e Próxima / Sobrepor / Falar
   sem baixar) + Stop / Loop / A± volume + an edit toggle. Pure controls — all
   state lives in useCartwall; this only renders + emits. */
export function CartwallToolbar({
  mode,
  onMode,
  loop,
  onToggleLoop,
  onStop,
  onLouder,
  onSofter,
  editing,
  onToggleEdit,
}: {
  mode: FireMode;
  onMode: (m: FireMode) => void;
  loop: boolean;
  onToggleLoop: () => void;
  onStop: () => void;
  onLouder: () => void;
  onSofter: () => void;
  editing: boolean;
  onToggleEdit: () => void;
}) {
  const modes: { key: FireMode; label: string }[] = [
    { key: "fade", label: "Fade e Próxima" },
    { key: "overlap", label: "Sobrepor sem fade" },
    { key: "talk", label: "Falar s/ baixar volume" },
  ];
  return (
    <div className="cw-foot">
      {modes.map((m) => (
        <label key={m.key} className="cw-opt">
          <input type="radio" name="cw-mode" checked={mode === m.key} onChange={() => onMode(m.key)} />
          {m.label}
        </label>
      ))}
      <span className="cw-spacer" />
      <button className="cw-fbtn" onClick={onStop}>
        ⏹ Stop
      </button>
      <button className={"cw-fbtn" + (loop ? " on" : "")} onClick={onToggleLoop}>
        🔁 Loop
      </button>
      <button className="cw-fbtn" onClick={onLouder}>
        A+ Maior
      </button>
      <button className="cw-fbtn" onClick={onSofter}>
        A- Menor
      </button>
      <button className={"cw-fbtn" + (editing ? " on" : "")} onClick={onToggleEdit}>
        ✎ Editar
      </button>
    </div>
  );
}
