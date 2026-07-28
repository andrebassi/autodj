"use client";

import { useEffect, useState } from "react";
import type { Deck } from "@/hooks/useDeck";

/* Output-device picker for the deck's cue (pre-listen) path. Enumerates audio
   outputs and drives <audio>.setSinkId via the hook, so the operator can monitor
   on headphones (a second device) while the master feeds the room. Devices only
   carry labels after a mediaDevices permission grant; before that they show as
   generic entries. CUE toggle sits here too — pre-listen is independent of the
   crossfader/master fade. setSinkId is Chromium-only; where absent the picker is
   inert but harmless. */
export function CueDeviceSelect({ d, disabled }: { d: Deck; disabled: boolean }) {
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const supported = typeof navigator !== "undefined" && !!navigator.mediaDevices?.enumerateDevices;

  useEffect(() => {
    if (!supported) return;
    let alive = true;
    const refresh = () =>
      navigator.mediaDevices
        .enumerateDevices()
        .then((list) => {
          if (alive) setDevices(list.filter((x) => x.kind === "audiooutput"));
        })
        .catch(() => {});
    refresh();
    navigator.mediaDevices.addEventListener?.("devicechange", refresh);
    return () => {
      alive = false;
      navigator.mediaDevices.removeEventListener?.("devicechange", refresh);
    };
  }, [supported]);

  return (
    <div className="cuebar">
      <button
        className={"cue-toggle" + (d.cueOn ? " on" : "")}
        disabled={disabled}
        onClick={() => d.toggleCue()}
        title="Pré-escuta (cue) — saída separada, ignora o crossfader"
      >
        CUE
      </button>
      <select
        className="cue-device"
        disabled={disabled || !supported || devices.length === 0}
        onChange={(e) => d.setCueDevice(e.target.value)}
        aria-label="Saída de pré-escuta"
        title={supported ? "Dispositivo de pré-escuta" : "setSinkId não suportado neste navegador"}
      >
        <option value="">saída padrão</option>
        {devices.map((dev, i) => (
          <option key={dev.deviceId || i} value={dev.deviceId}>
            {dev.label || `Saída ${i + 1}`}
          </option>
        ))}
      </select>
    </div>
  );
}
