"use client";

import { useState } from "react";
import { trackArtworkURL } from "@/lib/api";

/* Capa uniforme (hexagonal): recebe só o track_id e deriva a URL do endpoint
   /library/tracks/{id}/artwork — o servidor resolve por provider (Spotify via
   redirect da album image, local via ffmpeg do embedded picture, 404 quando não
   há). NÃO é caso especial por provider: a mesma peça vale p/ spotify/youtube/
   beatport/local. Sem id, ou onError (404 / faixa sem capa), cai no ladrilho
   placeholder — a UI nunca quebra por falta de arte.

   size: "deck" (player, grande) | "row" (fila/histórico, pequena) | "spot"
   (lista Spotify, média).

   src: URL de capa direta (Spotify search ainda não é faixa importada — não tem
   track_id/endpoint; a album image vem crua no CatalogItem). Quando dado, usa
   ele direto e ignora o trackId.

   onClick: torna a capa clicável (cursor zoom-in) — usado p/ abrir o popup de
   detalhes da faixa em deck/fila/tocadas/Mídias. stopPropagation evita disparar
   seleção/drag da linha por baixo. */
export function TrackCover({
  trackId,
  src,
  size = "row",
  alt = "",
  onClick,
}: {
  trackId?: string;
  src?: string;
  size?: "deck" | "row" | "spot";
  alt?: string;
  onClick?: () => void;
}) {
  const [broken, setBroken] = useState(false);
  const [srcFailed, setSrcFailed] = useState(false);
  const clickable = !!onClick;
  const onCoverClick = onClick
    ? (e: React.MouseEvent) => {
        e.stopPropagation();
        onClick();
      }
    : undefined;
  const cls =
    "track-cover cover-" + size + (clickable ? " cover-click" : "");

  // Preferência: capa direta (`src`, ex. CDN Spotify) — nunca depende do endpoint
  // sessão-scoped. `??` trocaria "" por si mesmo (nullish não pega string vazia),
  // então uso truthy: src vazio cai no endpoint. Se o CDN quebrar (`srcFailed`),
  // ainda tenta o endpoint como fallback antes de desistir p/ o placeholder.
  const endpoint = trackId ? trackArtworkURL(trackId) : "";
  const primary = srcFailed ? endpoint : src || endpoint;

  if (!primary || broken) {
    // ladrilho placeholder: nota musical no lugar da arte ausente. Clicável
    // também (abre detalhes) quando onClick é dado.
    return (
      <span
        className={cls + " cover-ph"}
        aria-hidden={!clickable}
        onClick={onCoverClick}
        title={clickable ? "Ver detalhes da faixa" : undefined}
      >
        ♪
      </span>
    );
  }
  return (
    <img
      className={cls}
      src={primary}
      alt={alt}
      loading="lazy"
      decoding="async"
      onClick={onCoverClick}
      title={clickable ? "Ver detalhes da faixa" : undefined}
      onError={() => {
        // capa direta falhou e há endpoint p/ tentar → troca; senão desiste.
        if (!srcFailed && src && endpoint) setSrcFailed(true);
        else setBroken(true);
      }}
    />
  );
}
