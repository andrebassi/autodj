"use client";

import { useEffect, useRef, useState } from "react";
import type { VizMode } from "@/hooks/useVizMode";

/* Engrenagem discreta (canto superior direito do par de decks). Abre um menu
   suspenso no MESMO padrão do clique-direito (.opctx) p/ trocar a visualização
   dos decks entre "linhas" (barra de progresso) e "waveform". Fecha ao clicar
   fora / rolar / Escape. */
const OPTS: { mode: VizMode; ico: string; label: string }[] = [
  { mode: "line", ico: "▬", label: "Linhas" },
  { mode: "wave", ico: "≈", label: "Waveform" },
];

export function VizMenu({
  mode,
  onPick,
}: {
  mode: VizMode;
  onPick: (m: VizMode) => void;
}) {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (e: Event) => {
      if (wrapRef.current && e.target instanceof Node && wrapRef.current.contains(e.target)) return;
      setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("mousedown", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", () => setOpen(false));
    window.addEventListener("keydown", esc);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("keydown", esc);
    };
  }, [open]);

  return (
    <div ref={wrapRef} className="vizmenu">
      <button
        type="button"
        className={"vizmenu-gear" + (open ? " active" : "")}
        onClick={() => setOpen((v) => !v)}
        title="Visualização dos decks (linhas / waveform)"
        aria-label="Visualização dos decks"
        aria-haspopup="menu"
        aria-expanded={open}
      >
        ⚙
      </button>
      {open && (
        <div className="opctx vizmenu-pop" role="menu" onMouseDown={(e) => e.stopPropagation()}>
          <div className="opctx-head">Visualização</div>
          {OPTS.map((o) => (
            <button
              key={o.mode}
              role="menuitemradio"
              aria-checked={mode === o.mode}
              className={"opctx-item" + (mode === o.mode ? " primary" : "")}
              onClick={() => {
                onPick(o.mode);
                setOpen(false);
              }}
            >
              <span className="opctx-ico">{o.ico}</span>
              {o.label}
              {mode === o.mode && <span className="vizmenu-check">✓</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
