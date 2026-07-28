"use client";

import { useEffect, useRef, useState } from "react";

/* uma ação de enfileirar: label + (pos, playNow) enviados ao backend */
export type EnqueueAction = { label: string; ico: string; pos: number; playNow: boolean };

export const ENQUEUE_ACTIONS: EnqueueAction[] = [
  { label: "Tocar agora", ico: "▶", pos: 0, playNow: true },
  { label: "Inserir após atual", ico: "⤵", pos: 0, playNow: false },
  { label: "Adicionar ao fim", ico: "＋", pos: -1, playNow: false },
];

/* menu flutuante (clique/botão direito) na lista de mídias.
   Enfileira + abre editor de marcações. Auto-clampa dentro da viewport. */
export function OpContextMenu({
  x,
  y,
  title,
  onPick,
  onOpenMarkers,
  onInstrumental,
  instrumental,
  onEdit,
  onDelete,
  onClose,
}: {
  x: number;
  y: number;
  title?: string;
  onPick: (a: EnqueueAction) => void;
  onOpenMarkers?: () => void;
  // separação de vocal por IA. Ausente = ferramenta não instalada → item some.
  onInstrumental?: () => void;
  instrumental?: "idle" | "queued" | "running" | "ready" | "failed";
  onEdit?: () => void;
  onDelete?: () => void;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y, ready: false });

  // fecha ao clicar fora / rolar / redimensionar
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

  // clampa dentro da janela depois de medir
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
      {ENQUEUE_ACTIONS.map((a) => (
        <button
          key={a.label}
          className={"opctx-item" + (a.playNow ? " primary" : "")}
          onClick={() => {
            onPick(a);
            onClose();
          }}
        >
          <span className="opctx-ico">{a.ico}</span>
          {a.label}
        </button>
      ))}
      {onOpenMarkers && (
        <>
          <div className="opctx-sep" />
          <button
            className="opctx-item"
            onClick={() => {
              onOpenMarkers();
              onClose();
            }}
          >
            <span className="opctx-ico">🎚</span>
            Editar marcações
          </button>
        </>
      )}
      {/* Separação de vocal por IA. Só aparece quando a ferramenta existe nesta
          máquina (`onInstrumental` ausente = escondido) — é opt-in e local.
          O rótulo diz o ESTADO, porque a operação leva minutos e o operador
          precisa saber se já pediu, se está rodando ou se já está pronta. */}
      {onInstrumental && (
        <>
          <div className="opctx-sep" />
          <button
            className="opctx-item"
            disabled={instrumental === "queued" || instrumental === "running"}
            onClick={() => {
              onInstrumental();
              onClose();
            }}
            title={
              instrumental === "ready"
                ? "Já preparada — o botão SEM-VOZ usa este arquivo"
                : "Gera uma versão sem vocal com IA. Leva alguns minutos e roda em segundo plano."
            }
          >
            <span className="opctx-ico">
              {instrumental === "ready" ? "✅" : instrumental === "failed" ? "⚠️" : "🎼"}
            </span>
            {instrumental === "ready"
              ? "SEM-VOZ pronta"
              : instrumental === "queued"
                ? "Na fila…"
                : instrumental === "running"
                  ? "Separando…"
                  : instrumental === "failed"
                    ? "Tentar separar de novo"
                    : "Processar SEM-VOZ (IA)"}
          </button>
        </>
      )}
      {(onEdit || onDelete) && <div className="opctx-sep" />}
      {onEdit && (
        <button
          className="opctx-item"
          onClick={() => {
            onEdit();
            onClose();
          }}
        >
          <span className="opctx-ico">✏️</span>
          Editar mídia
        </button>
      )}
      {onDelete && (
        <button
          className="opctx-item danger"
          onClick={() => {
            onDelete();
            onClose();
          }}
        >
          <span className="opctx-ico">🗑</span>
          Apagar mídia
        </button>
      )}
    </div>
  );
}
