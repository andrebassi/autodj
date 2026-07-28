"use client";

import { useEffect, useState } from "react";

/* Preferência da visualização do cabeçalho: linha de progresso REAL ou waveform
   cosmético. Mantém os dois; usuário alterna e a escolha PERSISTE no browser. */
export type VizMode = "line" | "wave";

const KEY = "autodj.vizmode.v1";

export function useVizMode(): [VizMode, () => void, (m: VizMode) => void] {
  const [mode, setMode] = useState<VizMode>("line");

  // hidrata do localStorage no cliente (default = linha).
  useEffect(() => {
    try {
      const v = window.localStorage.getItem(KEY);
      if (v === "wave" || v === "line") setMode(v);
    } catch {
      /* ignore */
    }
  }, []);

  const persist = (m: VizMode) => {
    try {
      window.localStorage.setItem(KEY, m);
    } catch {
      /* ignore */
    }
  };

  const set = (m: VizMode) => setMode(() => (persist(m), m));
  const cycle = () =>
    setMode((m) => {
      const next: VizMode = m === "line" ? "wave" : "line";
      persist(next);
      return next;
    });

  return [mode, cycle, set];
}
