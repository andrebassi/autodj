"use client";

import { useState } from "react";
import type { YouTubeItem } from "@/lib/api";
import { YouTubeResultMenu } from "@/components/YouTubeResultMenu";
import { TrackCover } from "@/components/TrackCover";
import { dragCover } from "@/lib/dragImage";

/* formata duração ms → m:ss (0 vira "—", caso de playlist/sem metadata). */
function fmtDur(ms: number): string {
  if (ms <= 0) return "—";
  const s = Math.round(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/* lista de resultados da busca YouTube — espelho fiel de SpotifyResults. Sem botão
   por linha: importar/enfileirar ficam no menu de botão direito (YouTubeResultMenu)
   e cada linha é arrastável (carrega a URI YouTube). A linha NÃO mostra selo de
   mixabilidade (0.2.14): toda faixa YouTube é não-mixável por definição, então o
   selo era ruído repetido em todas as linhas. */
export function YouTubeResults({
  results,
  busy,
  onImport,
  onEnqueue,
}: {
  results: YouTubeItem[];
  busy: boolean;
  onImport: (item: YouTubeItem) => void;
  onEnqueue?: (item: YouTubeItem, pos: number, playNow: boolean) => void;
}) {
  const [menu, setMenu] = useState<{ x: number; y: number; item: YouTubeItem } | null>(null);

  return (
    <ul className="spot-results">
      {results.map((it) => {
        return (
          <li
            key={it.uri}
            className="spot-row"
            draggable
            onDragStart={(e) => {
              e.dataTransfer.setData("text/autodj-youtube-uri", it.uri);
              e.dataTransfer.setData("text/plain", it.uri);
              // playlist arrastada → nome vira a origem das faixas na fila.
              if (it.kind === "playlist")
                e.dataTransfer.setData("text/autodj-youtube-origin", it.title);
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
              {it.kind === "playlist" ? "📃 Playlist" : "🎬 Vídeo"}
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
        <YouTubeResultMenu
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
