"use client";

import { useEffect, type RefObject } from "react";
import type { YouTubeHostRole } from "@/hooks/useYouTubePlayout";

/* Sobreposição do host do IFrame do YouTube no slot de capa do deck ao vivo SEM
   reparentar. CAUSA-RAIZ (v0.2.4): mover um <iframe> no DOM — `appendChild` para
   outro pai, como o Deck fazia — o RECARREGA (comportamento de spec do HTML: ao
   ser removido+reinserido no documento o iframe reinicia o content document).
   Isso matava o handshake postMessage do YT.Player a cada troca A/B → o vídeo
   nunca tocava (player virava um about:blank recém-recarregado). Prova empírica:
   um iframe reparentado dispara um 2º evento `load`.

   Solução: o host NUNCA se move na árvore (fica estacionado em page.tsx). Quando
   um deck YouTube está ao vivo, sobrepomos o host VISUALMENTE via position:fixed,
   casando o retângulo do slot ([data-yt-slot]) a cada frame. Reposicionar por CSS
   NÃO recarrega o iframe → o player sobrevive ao ping-pong A/B e toca.

   v0.2.11: há DOIS hosts (no ar + engatilhado/pré-carga). O papel escolhe o slot
   de destino — "live" → [data-yt-slot] (deck no ar), "cue" → [data-yt-cue-slot]
   (deck ocioso, faixa já bufferizada), null → parqueado. */
export function useYouTubeOverlay(
  hostRef: RefObject<HTMLDivElement | null>,
  role: YouTubeHostRole,
): void {
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    // volta o host ao estado estacionado (dentro do wrapper oculto de page.tsx).
    const park = () => {
      host.style.position = "";
      host.style.left = "";
      host.style.top = "";
      host.style.width = "100%";
      host.style.height = "100%";
      host.style.zIndex = "";
      host.style.pointerEvents = "";
      host.style.visibility = "";
    };

    if (!role) {
      park();
      return;
    }

    const sel = role === "live" ? "[data-yt-slot]" : "[data-yt-cue-slot]";
    let raf = 0;
    const tick = () => {
      const slot = document.querySelector<HTMLElement>(sel);
      host.style.position = "fixed";
      host.style.zIndex = "5";
      host.style.pointerEvents = "none";
      if (slot) {
        const r = slot.getBoundingClientRect();
        host.style.left = `${r.left}px`;
        host.style.top = `${r.top}px`;
        host.style.width = `${r.width}px`;
        host.style.height = `${r.height}px`;
        host.style.visibility = "visible";
      } else {
        // deck YouTube ativo mas slot ainda não montado — mantém o player vivo,
        // só invisível fora da tela (viewport mínimo preservado p/ não recusar).
        host.style.left = "-9999px";
        host.style.top = "0px";
        host.style.width = "60px";
        host.style.height = "60px";
        host.style.visibility = "hidden";
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => {
      cancelAnimationFrame(raf);
      park();
    };
  }, [hostRef, role]);
}
