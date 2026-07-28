"use client";

import { useEffect, useState } from "react";
import { apiBase } from "@/lib/api";
import { EMPTY_OPERATIONAL, type Operational } from "@/lib/uiContent";

/* Catálogo da tela Operacional (GET /api/v1/ui/operational) — rótulos, ícones e
   categorias. Renderiza vazio até resolver, pros componentes não esperarem.

   ESPELHO LOCAL (0.2.27): o catálogo é conteúdo estável (uns poucos KB de rótulos)
   e sem ele a tela abre SEM OS CHIPS de categoria — foi o que aconteceu num F5 sem
   servidor: o painel perdeu "Músicas/Spotify/YouTube" e caiu numa aba solta. Como
   o app agora sobrevive offline, o catálogo precisa sobreviver junto. */
const MIRROR = "autodj.operational.v1";

function readMirror(): Operational | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(MIRROR);
    const data = raw ? (JSON.parse(raw) as Operational) : null;
    return data?.categories ? data : null;
  } catch {
    return null;
  }
}

export function useOperational(): { op: Operational; loaded: boolean } {
  const [op, setOp] = useState<Operational>(() => readMirror() ?? EMPTY_OPERATIONAL);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const res = await fetch(apiBase() + "/api/v1/ui/operational");
        const data: Operational = await res.json();
        if (!stop && data?.categories) {
          setOp(data);
          try {
            window.localStorage.setItem(MIRROR, JSON.stringify(data));
          } catch {
            /* cota/privado — só perde o conforto offline */
          }
        }
      } catch {
        /* servidor fora: fica com o espelho carregado no estado inicial */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  return { op, loaded };
}
