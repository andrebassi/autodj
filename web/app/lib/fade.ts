"use client";

import { useSyncExternalStore } from "react";

/* Fade nas transições do Tocador (duração escolhida em ⚙ Configurações):
   - ligado  → PRÓXIMA/RETROCEDER descem o que toca em Xs e sobem a nova em Xs.
   - desligado→ corte seco (troca instantânea), como o STOP.

   Duração configurável: 0 (corte seco), 3s, 5s ou 7s. Store client-side
   (localStorage) — uso caseiro, é preferência de UI, não vale coluna no banco
   (mesmo padrão do modo, `mode.ts`). useSyncExternalStore mantém os hooks de
   playout em sincronia; os hooks também leem `fadeMs()`/`fadeEnabled()` na hora
   da transição (getter síncrono) p/ respeitar a escolha sem re-render.

   Default = 3s (o recurso nasceu ligado em 3s). Compat com o valor legado:
   "1" (ligado antigo) → 3000ms; "0" → corte seco; ausente → 3000ms. */

const KEY = "autodj.fade.v1";
const DEFAULT_MS = 3000;
export const FADE_OPTIONS_MS = [0, 3000, 5000, 7000] as const;
const listeners = new Set<() => void>();
let current: number | null = null;

function read(): number {
  if (typeof window === "undefined") return DEFAULT_MS;
  const raw = window.localStorage.getItem(KEY);
  if (raw === null) return DEFAULT_MS; // ausente = default
  if (raw === "0") return 0; // desligado (corte seco)
  if (raw === "1") return DEFAULT_MS; // legado "ligado" = 3s
  const ms = Number(raw);
  return Number.isFinite(ms) && ms > 0 ? ms : DEFAULT_MS;
}

/* duração do fade em ms (0 = corte seco). */
export function fadeMs(): number {
  if (current === null) current = read();
  return current;
}

export function fadeEnabled(): boolean {
  return fadeMs() > 0;
}

export function setFadeMs(ms: number): void {
  current = ms > 0 ? ms : 0;
  if (typeof window !== "undefined") {
    try {
      window.localStorage.setItem(KEY, String(current));
    } catch {
      /* quota — ignora */
    }
  }
  listeners.forEach((l) => l());
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

/* hook reativo: re-renderiza quem lê quando a duração muda. */
export function useFadeMs(): number {
  return useSyncExternalStore(subscribe, fadeMs, () => DEFAULT_MS);
}
