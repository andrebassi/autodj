"use client";

import { useState } from "react";
import { classify, fmtDur, KIND_ICON } from "@/lib/format";
import type { QueueItem } from "@/lib/types";
import { playableToMedia } from "@/lib/operational";
import { dragCover } from "@/lib/dragImage";
import { QueueRowMenu } from "@/components/QueueRowMenu";
import { TrackCover } from "@/components/TrackCover";
import { SourceBadge } from "@/components/SourceBadge";

/* one real server-queue row, colored by classified kind.
   Mostra procedência (origin: playlist/grade/evento) e a fonte (Spotify/local)
   — Spotify toca out-of-band pelo SDK; local vai pro bus PCM.
   Botão-direito abre menu p/ remover o item da fila (onRemove).

   Reordenar = ARRASTAR E SOLTAR (drag-and-drop). Pega pelo grip (⠿) e solta na
   posição desejada; o indicador (borda azul em cima/embaixo da linha alvo) mostra
   onde vai cair. `index` = posição VISÍVEL (na cauda slice(2)); `onReorder(from,to)`
   recebe índices visíveis — o ProgramList converte p/ índices do mirror (+2) e
   chama /playback/move. Usa um type de dataTransfer próprio (text/autodj-reorder)
   p/ NÃO colidir com o drop de mídia da biblioteca (text/autodj-track/-spotify-uri). */
export function QueueRow({
  item,
  index,
  pos,
  onRemove,
  onReorder,
  onOpenDetail,
  stem,
}: {
  item: QueueItem;
  index: number;
  pos?: number; // posição GLOBAL na ordem de execução (1=no ar, 2=cued, 3+=cauda).
  onRemove?: () => void;
  onReorder?: (from: number, to: number) => void;
  onOpenDetail?: (media: ReturnType<typeof playableToMedia>) => void;
  /* estado da separação DESTA faixa, vindo do estado vivo do servidor. Ausente =
     nada a mostrar (nunca pedida, ou ferramenta indisponível). */
  stem?: "queued" | "running" | "ready" | "failed";
}) {
  const kind = classify(item.playable.title);
  const src = item.playable.source;
  const origin = item.playable.origin ?? "";
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [over, setOver] = useState<null | "up" | "down">(null);
  const [dragging, setDragging] = useState(false);

  const onContextMenu = (e: React.MouseEvent) => {
    if (!onRemove) return;
    e.preventDefault();
    setMenu({ x: e.clientX, y: e.clientY });
  };

  // --- drag-and-drop reorder ---
  const onDragStart = (e: React.DragEvent) => {
    if (!onReorder) {
      e.preventDefault();
      return;
    }
    e.dataTransfer.setData("text/autodj-reorder", String(index));
    e.dataTransfer.effectAllowed = "move";
    dragCover(e); // arrasta só a capa (com efeito), não a linha inteira
    setDragging(true);
  };
  const onDragEnd = () => {
    setDragging(false);
    setOver(null);
  };
  const isReorderDrag = (e: React.DragEvent) =>
    e.dataTransfer.types.includes("text/autodj-reorder");
  const onDragOver = (e: React.DragEvent) => {
    if (!onReorder || !isReorderDrag(e)) return; // deixa o drop de mídia subir p/ a lista
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = "move";
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setOver(e.clientY < r.top + r.height / 2 ? "up" : "down");
  };
  const onDragLeave = () => setOver(null);
  const onDrop = (e: React.DragEvent) => {
    if (!onReorder || !isReorderDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    const raw = e.dataTransfer.getData("text/autodj-reorder");
    const half = over;
    setOver(null);
    if (raw === "") return;
    const from = Number(raw);
    if (from === index) return; // soltou em si mesmo → no-op
    // índice VISÍVEL final desejado (MoveAt posiciona o item nesse índice):
    // solta ANTES da linha alvo (up) ou DEPOIS (down), compensando o shift de
    // remoção quando arrasta de cima p/ baixo (from < index).
    let to: number;
    if (half === "down") to = from < index ? index : index + 1;
    else to = from < index ? index - 1 : index;
    if (to === from) return;
    onReorder(from, to);
  };

  const cls =
    `row q k-${kind}` +
    (dragging ? " q-dragging" : "") +
    (over === "up" ? " drop-up" : over === "down" ? " drop-down" : "");

  return (
    <li
      className={cls}
      onContextMenu={onContextMenu}
      draggable={!!onReorder}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      <span className="r-pos" aria-label={pos ? `Posição ${pos}` : undefined}>
        {pos ?? ""}
      </span>
      <span
        className="r-grip"
        title={onReorder ? "Arraste p/ reordenar" : undefined}
        aria-hidden="true"
      >
        ⠿
      </span>
      <span className="r-ico">{KIND_ICON[kind]}</span>
      <span className="r-check">✓</span>
      <TrackCover
        trackId={item.playable.track_id}
        src={item.playable.artwork_url}
        size="row"
        alt={item.playable.title}
        onClick={
          onOpenDetail
            ? () => onOpenDetail(playableToMedia(item.playable))
            : undefined
        }
      />
      <span className="r-title">
        <span className="r-name">
          {item.playable.title || item.playable.track_id}
          {/* SEM-VOZ pronto e preparo em andamento — a fila precisa disto tanto
              quanto a grade, e não pode ler do `Playable`: ele é um retrato do
              enfileiramento e a separação termina minutos depois. Vem do estado
              vivo (`useStemWork`), que o pai repassa. */}
          {stem === "ready" && (
            <span className="opmedia-stem" title="Versão SEM-VOZ pronta">
              🎼 SEM-VOZ
            </span>
          )}
        </span>
        {item.playable.artist && (
          <span className="r-artist">{item.playable.artist}</span>
        )}
        {(stem === "queued" || stem === "running") && (
          <span
            className={"opmedia-prep" + (stem === "queued" ? " queued" : "")}
            title={
              stem === "queued"
                ? "Na fila para separar a voz"
                : "Processando a voz (alguns minutos)"
            }
          >
            <i />
          </span>
        )}
      </span>
      <span className="r-src">
        <SourceBadge source={src} />
      </span>
      <span className="r-origin" title={origin ? `De: ${origin}` : undefined}>
        {origin || "—"}
      </span>
      {/* BPM viaja no Playable (é estático). Vazio quando não medido — melhor
          coluna em branco que um "—" repetido em toda linha. */}
      <span className="r-bpm">
        {item.playable.bpm ? Math.round(item.playable.bpm) : ""}
      </span>
      <span className="r-trans">{item.transition}</span>
      <span className="r-dur">{fmtDur(item.playable.duration_ms)}</span>
      {menu && onRemove && (
        <QueueRowMenu
          x={menu.x}
          y={menu.y}
          title={item.playable.title || item.playable.track_id}
          onRemove={onRemove}
          onClose={() => setMenu(null)}
        />
      )}
    </li>
  );
}
