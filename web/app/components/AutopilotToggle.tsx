"use client";

import { useAutopilot } from "@/hooks/useAutopilot";

/* piloto automático (Story 4.5) — botão na barra de título que liga/desliga a
   supervisão não-assistida. Verde = no ar (nunca fica mudo); mostra a fila viva.
   Toda a I/O vem do hook via API — sem estado hardcoded. */
export function AutopilotToggle() {
  const { status, busy, error, toggle } = useAutopilot();
  const on = status.enabled;

  return (
    <button
      className={"op-autopilot" + (on ? " on" : "")}
      onClick={() => toggle(!on)}
      disabled={busy}
      title={
        error
          ? "Piloto automático: " + error
          : on
            ? "Piloto automático LIGADO — fila: " + status.queueLen
            : "Piloto automático desligado"
      }
      aria-pressed={on}
      aria-label="Piloto automático"
    >
      <span className="op-autopilot-dot" />
      {on ? "AUTO" : "AUTO OFF"}
      {on && <span className="op-autopilot-q">{status.queueLen}</span>}
    </button>
  );
}
