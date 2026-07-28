"use client";

import { useEffect, useRef, useState } from "react";
import type { NowPlaying, Playable, PlayedItem } from "@/lib/types";

const KEY = "autodj.playhistory.v1";

/* dia local no formato YYYY-MM-DD — chave de expiração do histórico. */
function today(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(
    d.getDate(),
  ).padStart(2, "0")}`;
}

type Stored = { day: string; items: PlayedItem[] };

/* carimba a faixa com a hora local em que saiu do ar (virou tocada). */
function stamp(p: Playable): PlayedItem {
  return { ...p, played_at: Date.now() };
}

function load(): Stored {
  const day = today();
  if (typeof window === "undefined") return { day, items: [] };
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return { day, items: [] };
    const s = JSON.parse(raw) as Stored;
    // dia diferente → sessão de ontem, começa limpo (limpa geral ao virar o dia).
    if (!s || s.day !== day || !Array.isArray(s.items)) return { day, items: [] };
    return { day, items: s.items };
  } catch {
    return { day, items: [] };
  }
}

function persist(day: string, items: PlayedItem[]): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(KEY, JSON.stringify({ day, items } satisfies Stored));
  } catch {
    /* quota cheia — ignora */
  }
}

/* Histórico de tocadas (estilo RadioPro): observa o now-playing do WebSocket e
   acumula a faixa ANTERIOR sempre que a atual muda. Renderiza esmaecido logo
   acima do no-ar.

   PERSISTENTE no browser (localStorage), chaveado pelo dia: SOBREVIVE a refresh
   — a fila e o now-playing já voltam do servidor (WS), o histórico volta daqui.
   Limpa geral SÓ quando VIRA O DIA, nunca no reload. O PlayLog append-only do
   servidor continua sendo a fonte durável de "mais tocadas" (Épico 7). Mantém as
   últimas `limit`, mais recente por último. */
export function usePlayHistory(
  snap: NowPlaying | null,
  limit = 40,
): { items: PlayedItem[]; clear: () => void } {
  const boot = useRef<Stored>(load());
  const [hist, setHist] = useState<PlayedItem[]>(boot.current.items);
  const dayRef = useRef<string>(boot.current.day);
  // guarda a Playable atual entre snapshots — só temos o snapshot corrente, então
  // precisamos lembrar a anterior p/ empurrá-la ao histórico quando trocar.
  const prev = useRef<Playable | null>(null);

  useEffect(() => {
    const np = snap?.now_playing;
    const id = np && np.track_id ? np.track_id : "";
    const prevNp = prev.current;
    const prevId = prevNp?.track_id ?? "";

    // virou o dia com a sessão aberta → limpa geral e recomeça.
    const now = today();
    if (now !== dayRef.current) {
      dayRef.current = now;
      const fresh = prevNp && prevNp.track_id ? [stamp(prevNp)] : [];
      persist(now, fresh);
      setHist(fresh);
      prev.current = np && np.track_id ? np : null;
      return;
    }

    if (id === prevId) {
      // mesma faixa (WS reemite o snapshot) — atualiza a ref, sem tocar histórico
      if (np && np.track_id) prev.current = np;
      return;
    }
    // trocou de faixa (ou foi p/ "nada no ar"): a anterior, se real, virou tocada
    if (prevNp && prevNp.track_id) {
      setHist((h) => {
        const next = [...h, stamp(prevNp)].slice(-limit);
        persist(dayRef.current, next);
        return next;
      });
    }
    prev.current = np && np.track_id ? np : null;
  }, [snap, limit]);

  // zera o histórico agora (usado pelo "Limpar" do header): apaga o localStorage,
  // esvazia o estado e esquece a faixa-anterior pendente, pra não reaparecer.
  const clear = () => {
    if (typeof window !== "undefined") {
      try {
        window.localStorage.removeItem(KEY);
      } catch {
        /* ignora */
      }
    }
    prev.current = null;
    setHist([]);
  };

  return { items: hist, clear };
}
