"use client";

import { useEffect, useRef, useState } from "react";
import type { NowPlaying } from "@/lib/types";

/* Qual deck físico (A/B) está TOCANDO agora. Alterna a cada troca de faixa em vez
   de empurrar tudo sempre p/ o A: o deck ocioso recebe a próxima (cued); quando a
   atual acaba, ele passa a tocar e o outro fica ocioso e carrega a seguinte
   (ping-pong A↔B, como decks de DJ de verdade).

   A 1ª faixa da sessão fica no A; cada now-playing novo inverte o deck ativo. */
export function useActiveDeck(snap: NowPlaying | null): "A" | "B" {
  const [active, setActive] = useState<"A" | "B">("A");
  const prevId = useRef<string>("");

  useEffect(() => {
    const id = snap?.now_playing?.track_id ?? "";
    if (!id) return; // nada no ar → mantém o deck atual
    if (prevId.current === "") {
      prevId.current = id; // 1ª faixa fica em A
      return;
    }
    if (id !== prevId.current) {
      prevId.current = id;
      setActive((d) => (d === "A" ? "B" : "A"));
    }
  }, [snap]);

  return active;
}
