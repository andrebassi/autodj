"use client";

import { useState } from "react";
import type { OpMedia } from "@/lib/operational";
import { OpContextMenu, type EnqueueAction } from "@/components/OpContextMenu";
import { TrackCover } from "@/components/TrackCover";
import { TrackDetailModal } from "@/components/TrackDetailModal";

/* tabela de mídias — linha selecionada em verde. Itens com `id` (catálogo real)
   são arrastáveis p/ a fila do Player e abrem menu (clique OU botão direito). */
export function OpMediaList({
  items,
  selected,
  onSelect,
  onEnqueue,
  onOpenMarkers,
  onInstrumental,
  instrumentalOf,
  onEdit,
  onDelete,
  working = {},
}: {
  items: OpMedia[];
  selected: number;
  onSelect: (i: number) => void;
  onEnqueue?: (trackId: string, pos: number, playNow: boolean) => void;
  onOpenMarkers?: (i: number) => void;
  // separação de vocal por IA — ausente = ferramenta não instalada, item some.
  onInstrumental?: (i: number) => void;
  instrumentalOf?: (m: OpMedia) => "idle" | "queued" | "running" | "ready" | "failed";
  onEdit?: (i: number) => void;
  onDelete?: (i: number) => void;
  /* estado de separação por faixa, de UMA consulta ao servidor.
     ⚠️ O mapa traz também `ready` (a FILA precisa disso para marcar as preparadas),
     então quem desenha BARRA tem de filtrar por queued/running — tratar qualquer
     valor como "em andamento" deixava a faixa pronta com a barra girando para
     sempre. */
  working?: Record<string, string>;
}) {
  const [menu, setMenu] = useState<{ x: number; y: number; id: string; i: number } | null>(null);
  const [detail, setDetail] = useState<OpMedia | null>(null);

  const openMenu = (e: React.MouseEvent, m: OpMedia, i: number) => {
    if (!m.id) return;
    e.preventDefault();
    onSelect(i);
    setMenu({ x: e.clientX, y: e.clientY, id: m.id, i });
  };

  const pick = (a: EnqueueAction) => {
    if (menu && onEnqueue) onEnqueue(menu.id, a.pos, a.playNow);
  };

  const cur = menu ? items[menu.i] : null;
  // a coluna de BPM só existe se houver o que mostrar nela.
  const anyBPM = items.some((m) => m.source === "local" && (m.bpm ?? 0) > 0);

  return (
    <div className="opmedia">
      <div className="opmedia-count">Mídias ({items.length})</div>
      <div className="opmedia-scroll">
        <table className="opmedia-tbl">
          <thead>
            <tr>
              <th className="c-ico" />
              <th className="c-artist">Interprete</th>
              <th className="c-title">Título</th>
              {/* BPM só faz sentido em faixa local: streaming não tem PCM deste
                  lado, então nunca foi analisado. A coluna some inteira quando não
                  há nenhuma local na lista, em vez de ficar uma fileira de "—". */}
              {anyBPM && <th className="c-bpm">BPM</th>}
              <th className="c-dur">Duração</th>
            </tr>
          </thead>
          <tbody>
            {items.map((m, i) => {
              const drag = !!m.id && !!onEnqueue;
              return (
                <tr
                  key={(m.id ?? "") + m.artist + m.title}
                  className={
                    "opmedia-row" +
                    (i === selected ? " sel" : "") +
                    (m.source === "spotify" ? " spotsrc" : "") +
                    (drag ? " drag" : "")
                  }
                  draggable={drag}
                  onDragStart={(e) => {
                    if (!m.id) return;
                    e.dataTransfer.setData("text/autodj-track", m.id);
                    e.dataTransfer.effectAllowed = "copy";
                  }}
                  onClick={() => onSelect(i)}
                  onContextMenu={(e) => openMenu(e, m, i)}
                  title={drag ? "Botão direito p/ ações · arraste p/ a fila" : undefined}
                >
                  <td
                    className="c-ico c-cover"
                    title="Ver detalhes da faixa"
                    onClick={(e) => {
                      e.stopPropagation();
                      setDetail(m);
                    }}
                  >
                    <TrackCover trackId={m.id} size="row" alt={m.title} />
                  </td>
                  <td className="c-artist">{m.artist}</td>
                  <td className="c-title">
                    {/* título e selo na MESMA linha: o selo quebrando para baixo
                        deixava as linhas com alturas diferentes e a tabela
                        desalinhada. O título trunca; o selo nunca sai do lugar. */}
                    <span className="opmedia-name">{m.title}</span>
                    {/* Selo do instrumental preparado. Fica JUNTO ao título (e não
                        numa coluna própria) porque é atributo de poucas faixas: uma
                        coluna dedicada seria quase toda vazia. */}
                    {m.instrumentalReady && (
                      <span
                        className="opmedia-stem"
                        title="Versão SEM-VOZ preparada por IA — o botão SEM-VOZ liga nesta faixa"
                      >
                        SEM-VOZ
                      </span>
                    )}
                    {/* Barra INDETERMINADA, ABAIXO do nome: ela pertence à faixa
                        inteira, não ao fim do título — ao lado do texto ela
                        competia com o nome e a largura dependia do tamanho dele.
                        O modelo não reporta progresso que dê pra confiar, então
                        ela diz "está acontecendo", não "falta tanto". Aparece ao
                        começar e some ao terminar. */}
                    {(working[m.id ?? ""] === "queued" ||
                      working[m.id ?? ""] === "running") && (
                      <span
                        className={
                          "opmedia-prep" +
                          (working[m.id ?? ""] === "queued" ? " queued" : "")
                        }
                        title={
                          working[m.id ?? ""] === "queued"
                            ? "Na fila para separar a voz"
                            : "Separando a versão sem voz (alguns minutos)"
                        }
                      >
                        <i />
                      </span>
                    )}
                  </td>
                  {anyBPM && (
                    <td className="c-bpm">
                      {m.bpm ? Math.round(m.bpm) : ""}
                    </td>
                  )}
                  <td className="c-dur">
                    {m.dur}
                    {m.markers && <span className="opmedia-mk">{m.markers}</span>}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {menu && (
        <OpContextMenu
          x={menu.x}
          y={menu.y}
          title={cur ? `${cur.title} — ${cur.artist}` : undefined}
          onPick={pick}
          onOpenMarkers={onOpenMarkers ? () => onOpenMarkers(menu.i) : undefined}
          // só faixa LOCAL: streaming toca out-of-band, não há PCM pra separar
          onInstrumental={
            onInstrumental && cur?.source === "local" ? () => onInstrumental(menu.i) : undefined
          }
          instrumental={cur && instrumentalOf ? instrumentalOf(cur) : undefined}
          onEdit={onEdit ? () => onEdit(menu.i) : undefined}
          onDelete={onDelete ? () => onDelete(menu.i) : undefined}
          onClose={() => setMenu(null)}
        />
      )}

      {detail && (
        <TrackDetailModal media={detail} onClose={() => setDetail(null)} />
      )}
    </div>
  );
}
