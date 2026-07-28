"use client";

import { useState } from "react";
import type { CatalogItem } from "@/lib/api";
import { SpotifyResultMenu } from "@/components/SpotifyResultMenu";
import { TrackCover } from "@/components/TrackCover";
import { dragCover } from "@/lib/dragImage";

/* formata duração ms → m:ss (0 vira "—", caso de playlist/sem metadata). */
function fmtDur(ms: number): string {
  if (ms <= 0) return "—";
  const s = Math.round(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/* lista de resultados da busca Spotify. Sem botão por linha: importar/enfileirar
   ficam no menu de botão direito (SpotifyResultMenu) e cada linha é arrastável
   (carrega o URI Spotify). A linha NÃO mostra selo de mixabilidade (0.2.14): toda
   faixa Spotify é não-mixável por definição, então o selo era ruído repetido em
   todas as linhas. */
export function SpotifyResults({
  results,
  busy,
  onImport,
  onEnqueue,
}: {
  results: CatalogItem[];
  busy: boolean;
  onImport: (item: CatalogItem) => void;
  onEnqueue?: (item: CatalogItem, pos: number, playNow: boolean) => void;
}) {
  const [menu, setMenu] = useState<{ x: number; y: number; item: CatalogItem } | null>(null);

  return (
    <ul className="spot-results">
      {results.map((it) => {
        return (
          <li
            key={it.uri}
            className="spot-row"
            draggable
            onDragStart={(e) => {
              e.dataTransfer.setData("text/autodj-spotify-uri", it.uri);
              e.dataTransfer.setData("text/plain", it.uri);
              // playlist arrastada → nome vira a origem das faixas na fila.
              if (it.kind === "playlist")
                e.dataTransfer.setData("text/autodj-spotify-origin", it.title);
              e.dataTransfer.effectAllowed = "copy";
              dragCover(e); // arrasta só a capa (com efeito), não a linha
            }}
            onContextMenu={(e) => {
              e.preventDefault();
              setMenu({ x: e.clientX, y: e.clientY, item: it });
            }}
            title="Botão direito p/ enfileirar/importar · arraste p/ o playout (importa automático)"
          >
            <span className="spot-kind">
              {it.kind === "playlist" ? "📃 Playlist" : "🎵 Faixa"}
            </span>
            <TrackCover src={it.artworkUrl} size="spot" alt={it.title} />
            <span className="spot-meta">
              <b className="spot-title">{it.title}</b>
              <span className="spot-artist">{it.artist}</span>
            </span>
            <span className="spot-dur">{fmtDur(it.durationMs)}</span>
          </li>
        );
      })}

      {menu && (
        <SpotifyResultMenu
          x={menu.x}
          y={menu.y}
          item={menu.item}
          busy={busy}
          onImport={onImport}
          onEnqueue={onEnqueue}
          onClose={() => setMenu(null)}
        />
      )}
    </ul>
  );
}
