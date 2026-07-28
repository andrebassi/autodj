"use client";

import { useSyncExternalStore } from "react";

/* Modo do player (escolhido no modal "Modo", abaixo do Tema):
   - "radio"   → estação completa: relógios/grade/eventos/RDS/ECAD/pedidos/… + todas
                 as categorias de mídia (vinhetas/trilhas/comerciais/…).
   - "tocador" → tocador puro de música: esconde as funções de rádio e as categorias
                 de estação, deixando só o essencial pra ouvir/enfileirar música.

   Store client-side (localStorage) — uso caseiro, é toggle de UI, não vale coluna no
   banco. useSyncExternalStore sincroniza todos os componentes que leem o modo (o
   modal grava, a tela Operacional reage na hora). Carimba <html data-mode> pra CSS.
   Default = "tocador" (uso caseiro = tocador puro; "radio" só quando o operador liga). */

export type PlayerMode = "radio" | "tocador";

const KEY = "autodj.mode.v1";
const listeners = new Set<() => void>();
let current: PlayerMode | null = null;

function read(): PlayerMode {
  if (typeof window === "undefined") return "tocador";
  const v = window.localStorage.getItem(KEY);
  return v === "radio" ? "radio" : "tocador";
}

function stamp(m: PlayerMode): void {
  if (typeof document !== "undefined") document.documentElement.dataset.mode = m;
}

export function getMode(): PlayerMode {
  if (current === null) {
    current = read();
    stamp(current);
  }
  return current;
}

export function setMode(m: PlayerMode): void {
  current = m;
  if (typeof window !== "undefined") {
    try {
      window.localStorage.setItem(KEY, m);
    } catch {
      /* quota — ignora */
    }
  }
  stamp(m);
  listeners.forEach((l) => l());
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

/* hook reativo: re-renderiza quem lê quando o modo muda. */
export function useMode(): PlayerMode {
  return useSyncExternalStore(subscribe, getMode, () => "tocador");
}
