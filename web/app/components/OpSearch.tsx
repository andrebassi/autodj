"use client";

import { useEffect, useRef } from "react";
import { useMode } from "@/lib/mode";

/* busca rápida — barra ampla e destacada. Filtra por intérprete/título.
   Atalho "/" foca o campo; Esc limpa/desfoca; ✕ limpa. */
export function OpSearch({
  value,
  onChange,
  count,
  total,
}: {
  value: string;
  onChange: (v: string) => void;
  count: number;
  total: number;
}) {
  const ref = useRef<HTMLInputElement>(null);
  const active = value.trim().length > 0;
  const empty = active && count === 0;
  // Tocador não tem vinheta/comercial (conceitos de estação) — placeholder enxuto.
  const ph =
    useMode() === "tocador"
      ? "Buscar música, artista, álbum…"
      : "Buscar música, vinheta, comercial, artista…";

  // "/" foca o campo de qualquer lugar (padrão de apps modernos)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = document.activeElement;
      const typing = el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA");
      if (e.key === "/" && !typing) {
        e.preventDefault();
        ref.current?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className={"opsearch" + (active ? " on" : "") + (empty ? " empty" : "")}>
      <input
        ref={ref}
        className="opsearch-input"
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            if (value) onChange("");
            else ref.current?.blur();
          }
        }}
        placeholder={ph}
        autoComplete="off"
        spellCheck={false}
      />

      {active ? (
        <>
          <span className={"opsearch-count" + (empty ? " zero" : "")}>
            {empty ? "nada encontrado" : `${count} de ${total}`}
          </span>
          <button className="opsearch-clear" onClick={() => onChange("")} title="Limpar (Esc)">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round">
              <line x1="6" y1="6" x2="18" y2="18" />
              <line x1="18" y1="6" x2="6" y2="18" />
            </svg>
          </button>
        </>
      ) : (
        <kbd className="opsearch-kbd">/</kbd>
      )}
    </div>
  );
}
