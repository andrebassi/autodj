"use client";

import { useEffect, useRef, useState } from "react";

/* menu flutuante do botão-direito numa linha da fila: uma ação — remover o item
   que o operador não quer. Reusa as classes .opctx e auto-clampa na viewport. */
export function QueueRowMenu({
  x,
  y,
  title,
  onRemove,
  onClose,
}: {
  x: number;
  y: number;
  title?: string;
  onRemove: () => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y, ready: false });

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

  useEffect(() => {
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
      {title && <div className="opctx-head">{title}</div>}
      <button
        className="opctx-item danger"
        onClick={() => {
          onRemove();
          onClose();
        }}
      >
        <span className="opctx-ico">🗑</span>
        Remover da fila
      </button>
    </div>
  );
}
