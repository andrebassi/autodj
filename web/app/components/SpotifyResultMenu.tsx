"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { CatalogItem } from "@/lib/api";
import { ENQUEUE_ACTIONS } from "@/components/OpContextMenu";

/* menu flutuante (botão direito) num resultado da busca Spotify. É o MESMO menu
   do local: as ações de enfileirar (Tocar agora / Inserir após / Adicionar ao
   fim) importam a faixa automaticamente e a enfileiram no playout (toca
   out-of-band pelo SDK, hard_cut). Abaixo, ainda dá pra só importar (metadata
   não-mixável). Auto-clampa dentro da viewport, igual ao OpContextMenu. */
export function SpotifyResultMenu({
  x,
  y,
  item,
  busy,
  onImport,
  onEnqueue,
  onClose,
}: {
  x: number;
  y: number;
  item: CatalogItem;
  busy: boolean;
  onImport: (item: CatalogItem) => void;
  onEnqueue?: (item: CatalogItem, pos: number, playNow: boolean) => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y, ready: false });
  const isPlaylist = item.kind === "playlist";

  useEffect(() => {
    const close = () => onClose();
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("mousedown", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("resize", close);
    window.addEventListener("keydown", esc);
    return () => {
      window.removeEventListener("mousedown", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("resize", close);
      window.removeEventListener("keydown", esc);
    };
  }, [onClose]);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const left = Math.min(x, window.innerWidth - r.width - 8);
    const top = Math.min(y, window.innerHeight - r.height - 8);
    setPos({ left: Math.max(8, left), top: Math.max(8, top), ready: true });
  }, [x, y]);

  return (
    <div
      ref={ref}
      className="opctx"
      style={{ left: pos.left, top: pos.top, visibility: pos.ready ? "visible" : "hidden" }}
      onMouseDown={(e) => e.stopPropagation()}
      onContextMenu={(e) => e.preventDefault()}
    >
      <div className="opctx-head">
        {item.title}
        {item.artist ? ` — ${item.artist}` : ""}
      </div>
      {onEnqueue &&
        ENQUEUE_ACTIONS.map((a) => (
          <button
            key={a.label}
            className={"opctx-item" + (a.playNow ? " primary" : "")}
            disabled={busy}
            onClick={() => {
              onEnqueue(item, a.pos, a.playNow);
              onClose();
            }}
          >
            <span className="opctx-ico">{a.ico}</span>
            {a.label}
          </button>
        ))}
      {onEnqueue && <div className="opctx-sep" />}
      <button
        className={"opctx-item" + (onEnqueue ? "" : " primary")}
        disabled={busy}
        onClick={() => {
          onImport(item);
          onClose();
        }}
      >
        <span className="opctx-ico">＋</span>
        {isPlaylist ? "Importar playlist" : "Importar para biblioteca"}
      </button>
    </div>
  );
}
