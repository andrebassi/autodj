import { useState } from "react";
import type { Playable, PlayedItem, QueueItem } from "@/lib/types";
import type { PlayoutDeck } from "@/hooks/useSpotifyPlayout";
import type { CrossMixState } from "@/hooks/useCrossEngineMix";
import type { VizMode } from "@/hooks/useVizMode";
import type { OpMedia } from "@/lib/operational";
import type { MixConfig } from "@/lib/uiContent";
import { useProgram } from "@/hooks/useProgram";
import { DeckPair } from "@/components/DeckPair";
import { LiveAnnounceRow } from "@/components/LiveAnnounceRow";
import { PlayoutTabs } from "@/components/PlayoutTabs";
import { TrackDetailModal } from "@/components/TrackDetailModal";

/* Playout do tocador: os DOIS decks (A/B) ficam FIXOS no topo (não rolam) e,
   abaixo, as abas TOCADAS | FILA rolam por dentro (ver PlayoutTabs). Antes tudo
   morava numa lista única rolável (histórico → decks → fila), o que fazia os
   decks "pularem" ao rolar; agora eles ficam parados e só as listas rolam.
   Drop SOBRE um deck (adiantar/cortar) continua aqui; append à fila mora na aba. */
export function ProgramList({
  hasNow,
  np,
  queue,
  history,
  announcing,
  wall,
  spotDeck,
  mix,
  activeDeck = "A",
  viz = "line",
  onPickViz,
  onSeek,
  onToggle,
  onEnqueue,
  onEnqueueSpotify,
  onEnqueueYouTube,
  onDequeue,
  onMove,
  onPromote,
  onPlayCued,
  stopped = false,
  onPlayMaster,
  onStopMaster,
  mixCfg,
  loopArmed = false,
  vocalCut = false,
}: {
  hasNow: boolean;
  np: Playable | undefined;
  queue: QueueItem[];
  history?: PlayedItem[];
  announcing: boolean;
  wall: string;
  spotDeck?: PlayoutDeck;
  mix?: CrossMixState; // janela da mixagem cross-motor (decks em MIX)
  activeDeck?: "A" | "B"; // qual deck físico está tocando (alterna por faixa)
  viz?: VizMode; // visualização dos decks: linha de progresso ou waveform
  onPickViz?: (m: VizMode) => void; // gear no par de decks troca a visualização
  onSeek?: (frac: number) => void;
  onToggle?: () => void; // play/pause do deck ativo
  stopped?: boolean; // estação parada — botão do deck ao vivo vira PLAY mestre
  onPlayMaster?: () => void; // ▶ mestre (deck ao vivo, parado) → estação no ar
  onStopMaster?: () => void; // ⏹ mestre (deck ao vivo, no ar) → estação fora do ar
  onEnqueue?: (trackId: string, pos: number, playNow: boolean) => void;
  onEnqueueSpotify?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onEnqueueYouTube?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onDequeue?: (pos: number, label?: string) => void; // botão-direito → remover da fila
  onMove?: (from: number, to: number) => void; // ↑/↓ → reordenar a ordem de execução
  // adiantar item JÁ na fila arrastando-o sobre um deck. `fromVisible` = índice
  // na cauda visível (slice(2)); `cut` = soltou no deck ao vivo (toca já) vs
  // ocioso (vira próxima). Backend: /playback/move (+ /playback/next se cut).
  onPromote?: (fromVisible: number, cut: boolean) => void;
  // ▶ do deck ocioso: promove a próxima carregada para o ar agora.
  onPlayCued?: () => void;
  // configuração de áudio das faixas locais — decide quais marcas o deck desenha
  mixCfg?: MixConfig;
  loopArmed?: boolean; // LOOP segurando a base agora (pulsa a faixa do loop)
  vocalCut?: boolean; // modo VOZ ligado (selo INSTRUMENTAL no deck que está no ar)
}) {
  const { above, below } = useProgram();
  // popup de detalhes da faixa — clicar na capa (deck/fila/tocadas) abre aqui.
  const [detail, setDetail] = useState<OpMedia | null>(null);
  // deslocamento p/ numeração ABSOLUTA: nº de faixas já tocadas. Deck ao vivo =
  // posBase+1, cued = posBase+2, cauda[i] = posBase+3+i, tocada[i] = i+1.
  const posBase = history?.length ?? 0;

  // drop SOBRE um deck. `deckIsLive` = soltou no deck TOCANDO agora:
  //  • ao vivo (cut) → tocar já (cabeça + playNow; item da fila → move+next);
  //  • ocioso/nada no ar (cue) → vira PRÓXIMA e espera (sem playNow/popup).
  const onDeckDrop = (e: React.DragEvent, deckIsLive: boolean) => {
    const reorder = e.dataTransfer.getData("text/autodj-reorder");
    if (reorder && onPromote) {
      void Promise.resolve(onPromote(Number(reorder), deckIsLive)).catch(() => {});
      return;
    }
    const cuePos = hasNow ? 1 : 0; // "coloca e espera": próxima, ou cabeça se nada no ar
    const uri = e.dataTransfer.getData("text/autodj-spotify-uri");
    if (uri && onEnqueueSpotify) {
      const origin = e.dataTransfer.getData("text/autodj-spotify-origin");
      void Promise.resolve(
        onEnqueueSpotify(uri, deckIsLive ? 0 : cuePos, deckIsLive, origin),
      ).catch(() => {});
      return;
    }
    const ytUri = e.dataTransfer.getData("text/autodj-youtube-uri");
    if (ytUri && onEnqueueYouTube) {
      const origin = e.dataTransfer.getData("text/autodj-youtube-origin");
      void Promise.resolve(
        onEnqueueYouTube(ytUri, deckIsLive ? 0 : cuePos, deckIsLive, origin),
      ).catch(() => {});
      return;
    }
    const id = e.dataTransfer.getData("text/autodj-track");
    if (id && onEnqueue) {
      void Promise.resolve(onEnqueue(id, deckIsLive ? 0 : cuePos, deckIsLive)).catch(
        () => {},
      );
    }
  };

  return (
    <div className="playout">
      {/* DECKS FIXOS — não rolam com as listas */}
      <ul className="rows deck-fixed">
        {announcing ? (
          <LiveAnnounceRow up="00:00" down="00:00" wall={wall} />
        ) : (
          <DeckPair
            hasNow={hasNow}
            np={np}
            queue={queue}
            posBase={posBase}
            livePosMs={spotDeck?.posMs}
            liveDurMs={spotDeck?.durMs}
            liveActive={!!spotDeck?.live}
            mix={mix}
            viz={viz}
            onPickViz={onPickViz}
            onSeek={onSeek}
            onToggle={onToggle}
            playing={!!spotDeck?.playing}
            stopped={stopped}
            onPlayMaster={onPlayMaster}
            onStopMaster={onStopMaster}
            activeDeck={activeDeck}
            canDrop={!!onEnqueue || !!onEnqueueSpotify || !!onEnqueueYouTube || !!onPromote}
            onDeckDrop={onDeckDrop}
            onPlayCued={onPlayCued}
            onOpenDetail={setDetail}
            mixCfg={mixCfg}
            loopArmed={loopArmed}
            vocalCut={vocalCut}
          />
        )}
      </ul>

      {/* ABAS roláveis: Tocadas | Fila */}
      <PlayoutTabs
        queue={queue}
        history={history}
        posBase={posBase}
        above={above}
        below={below}
        hasNow={hasNow}
        onEnqueue={onEnqueue}
        onEnqueueSpotify={onEnqueueSpotify}
        onEnqueueYouTube={onEnqueueYouTube}
        onDequeue={onDequeue}
        onMove={onMove}
        onOpenDetail={setDetail}
      />

      {detail && (
        <TrackDetailModal media={detail} onClose={() => setDetail(null)} />
      )}
    </div>
  );
}
