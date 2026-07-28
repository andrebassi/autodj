"use client";

import { useState, type CSSProperties } from "react";
import type { PlayedItem, QueueItem } from "@/lib/types";
import type { OpMedia } from "@/lib/operational";
import type { RichRow } from "@/lib/mockProgram";
import { RichRowView } from "@/components/RichRowView";
import { HistoryRow } from "@/components/HistoryRow";
import { useStemWork } from "@/hooks/useStemWork";
import { QueueRow } from "@/components/QueueRow";
import { EmptyQueue } from "@/components/EmptyQueue";

/* Abas abaixo dos decks fixos: TOCADAS (histórico da sessão + eventos passados) e
   FILA (próximas engatilhadas + eventos futuros). Só os decks A/B ficam no topo
   parados; estas listas rolam por dentro do próprio painel (`.grid`). O drop de
   mídia (append à fila) vive na aba FILA — arrastar da biblioteca cai aqui. */
type Tab = "fila" | "tocadas";

export function PlayoutTabs({
  queue,
  history,
  posBase = 0,
  above,
  below,
  hasNow,
  onEnqueue,
  onEnqueueSpotify,
  onEnqueueYouTube,
  onDequeue,
  onMove,
  onOpenDetail,
}: {
  queue: QueueItem[];
  history?: PlayedItem[];
  posBase?: number; // nº de tocadas — deslocamento p/ numeração absoluta da fila
  above: RichRow[];
  below: RichRow[];
  hasNow: boolean;
  onEnqueue?: (trackId: string, pos: number, playNow: boolean) => void;
  onEnqueueSpotify?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onEnqueueYouTube?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onDequeue?: (pos: number, label?: string) => void;
  onMove?: (from: number, to: number) => void;
  onOpenDetail?: (media: OpMedia) => void; // clicar na capa → popup de detalhes
}) {
  /* estado vivo da separação, do store único (mesma fonte da grade e do
     transporte) — o `Playable` não pode carregá-lo: é retrato do enfileiramento e
     o preparo termina minutos depois. */
  const { work: stemWork } = useStemWork();
  const [tab, setTab] = useState<Tab>("fila");

  // fila visível = tudo depois dos dois decks (queue[0]=A no ar, queue[1]=B próxima)
  const tail = queue.slice(2);
  const played = history ?? [];
  // quantas vezes cada faixa tocou na sessão — badge de "repetida" na aba Tocadas
  // (aparece quando >1, ex.: depois de RETROCEDER religar a mesma faixa).
  const playCounts = played.reduce((m, h) => {
    m.set(h.track_id, (m.get(h.track_id) ?? 0) + 1);
    return m;
  }, new Map<string, number>());
  const glide: CSSProperties = {
    width: "calc((100% - 8px) / 2)",
    transform: `translateX(${tab === "fila" ? 0 : 100}%)`,
  };

  // append à fila (drop de mídia local/Spotify) — só na aba FILA
  const onDragOver = (e: React.DragEvent) => {
    const t = e.dataTransfer.types;
    const localOk = onEnqueue && t.includes("text/autodj-track");
    const spotOk = onEnqueueSpotify && t.includes("text/autodj-spotify-uri");
    const ytOk = onEnqueueYouTube && t.includes("text/autodj-youtube-uri");
    if (!localOk && !spotOk && !ytOk) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
  };
  const onDrop = (e: React.DragEvent) => {
    const uri = e.dataTransfer.getData("text/autodj-spotify-uri");
    if (uri && onEnqueueSpotify) {
      e.preventDefault();
      const origin = e.dataTransfer.getData("text/autodj-spotify-origin");
      void Promise.resolve(onEnqueueSpotify(uri, -1, false, origin)).catch(() => {});
      return;
    }
    const ytUri = e.dataTransfer.getData("text/autodj-youtube-uri");
    if (ytUri && onEnqueueYouTube) {
      e.preventDefault();
      const origin = e.dataTransfer.getData("text/autodj-youtube-origin");
      void Promise.resolve(onEnqueueYouTube(ytUri, -1, false, origin)).catch(() => {});
      return;
    }
    const id = e.dataTransfer.getData("text/autodj-track");
    if (id && onEnqueue) {
      e.preventDefault();
      void Promise.resolve(onEnqueue(id, -1, false)).catch(() => {});
    }
  };

  return (
    <div className="ptabs">
      <div className="opsub2 ptabs-nav" role="tablist" aria-label="Tocadas e fila">
        <span className="opsub2-glide" style={glide} aria-hidden />
        <button
          type="button"
          role="tab"
          aria-selected={tab === "fila"}
          className={"opsub2-tab" + (tab === "fila" ? " on" : "")}
          onClick={() => setTab("fila")}
        >
          <span className="opsub2-ico">📋</span>
          <span className="opsub2-lbl">Fila{tail.length ? ` (${tail.length})` : ""}</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === "tocadas"}
          className={"opsub2-tab" + (tab === "tocadas" ? " on" : "")}
          onClick={() => setTab("tocadas")}
        >
          <span className="opsub2-ico">🕑</span>
          <span className="opsub2-lbl">Tocadas{played.length ? ` (${played.length})` : ""}</span>
        </button>
      </div>

      {tab === "fila" ? (
        <section className="grid" onDragOver={onDragOver} onDrop={onDrop}>
          <ul className="rows">
            {tail.map((it, i) => (
              <QueueRow
                key={it.playable.track_id + i}
                item={it}
                index={i}
                // numeração ABSOLUTA: no ar=base+1, cued (Deck B)=base+2, cauda
                // começa em base+3 (base = nº de tocadas). Reordenar re-renumera
                // sozinho (pos deriva do índice, não do item).
                pos={posBase + i + 3}
                stem={
                  stemWork[it.playable.track_id ?? ""] as
                    | "queued"
                    | "running"
                    | "ready"
                    | undefined
                }
                // visível i → índice no mirror = i+2 (pula Deck A e Deck B)
                onRemove={onDequeue ? () => onDequeue(i + 2, it.playable.title) : undefined}
                onReorder={onMove ? (from, to) => onMove(from + 2, to + 2) : undefined}
                onOpenDetail={onOpenDetail}
              />
            ))}
            {below.map((r, i) => (
              <RichRowView key={`b-${i}`} r={r} />
            ))}
            {tail.length === 0 && below.length === 0 && <EmptyQueue />}
          </ul>
        </section>
      ) : (
        <section className="grid">
          <ul className="rows">
            {above.map((r, i) => (
              <RichRowView key={`a-${i}`} r={r} />
            ))}
            {played.map((h, i) => (
              <HistoryRow
                key={h.played_at ?? `h-${h.track_id}-${i}`}
                item={h}
                // histórico é mais-antigo-primeiro → tocada[i] = posição i+1
                pos={i + 1}
                plays={playCounts.get(h.track_id)}
                onOpenDetail={onOpenDetail}
              />
            ))}
            {played.length === 0 && above.length === 0 && (
              <li className="grid-empty">Nada tocado ainda nesta sessão.</li>
            )}
          </ul>
        </section>
      )}
    </div>
  );
}
