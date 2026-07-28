"use client";

import { useMemo, useState } from "react";
import type { NowPlaying, Playable, PlayedItem } from "@/lib/types";
import { nowNext } from "@/lib/nowNext";
import { NowPlayingHeader } from "@/components/NowPlayingHeader";
import { NowViz } from "@/components/NowViz";
import { Timeline } from "@/components/Timeline";
import { ProgramList } from "@/components/ProgramList";
import { Transport } from "@/components/Transport";
import { useStemWork } from "@/hooks/useStemWork";
import { prepareInstrumental } from "@/lib/api";
import { toast } from "@/lib/toast";
import { LiveAnnounceModal } from "@/components/LiveAnnounceModal";
import { ToastHost } from "@/components/ToastHost";
import type { PlayoutDeck } from "@/hooks/useSpotifyPlayout";
import type { CrossMixState } from "@/hooks/useCrossEngineMix";
import { useActiveDeck } from "@/hooks/useActiveDeck";
import { useVizMode } from "@/hooks/useVizMode";
import { useMode } from "@/lib/mode";
import type { MixConfig } from "@/lib/uiContent";

/* Player screen (operador) — grade ao vivo + transport + locução */
export function PlayerScreen({
  now,
  snap,
  history,
  connected,
  busy,
  post,
  spotDeck,
  mix,
  onSpotToggle,
  onSpotSeek,
  wantPlay = true,
  onPlayMaster,
  onStopMaster,
  onEnqueue,
  onEnqueueSpotify,
  onEnqueueYouTube,
  onDequeue,
  onMove,
  onPromote,
  onPlayCued,
  onPrev,
  onNext,
  loopArmed = false,
  onLoopArmed,
  vocalCut = false,
  onVocalCut,
  mixCfg,
}: {
  now: Date | null;
  snap: NowPlaying | null;
  history?: PlayedItem[];
  connected: boolean;
  busy: boolean;
  post: (path: string) => void;
  spotDeck?: PlayoutDeck;
  // janela da mixagem cross-motor (Spotify↔YouTube): enquanto ela corre, a
  // entrante JÁ está tocando embora o now-playing do servidor ainda seja a que
  // sai — os decks usam isto p/ mostrar as duas em MIX com o tempo andando.
  mix?: CrossMixState;
  onSpotToggle?: () => void;
  onSpotSeek?: (frac: number) => void;
  wantPlay?: boolean; // PLAY mestre ligado? false = "nada no ar" (parado)
  onPlayMaster?: () => void; // Transport PLAY → estação no ar
  onStopMaster?: () => void; // Transport STOP → estação fora do ar
  onEnqueue?: (trackId: string, pos: number, playNow: boolean) => void;
  onEnqueueSpotify?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onEnqueueYouTube?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onDequeue?: (pos: number, label?: string) => void;
  onMove?: (from: number, to: number) => void;
  onPromote?: (fromVisible: number, cut: boolean) => void;
  onPlayCued?: () => void; // ▶ do deck ocioso → próxima carregada entra no ar
  onPrev?: () => void; // RETROCEDER → religa a última faixa das Tocadas ao ar
  onNext?: () => void; // PRÓXIMA → fade-out do atual + avança a fila
  loopArmed?: boolean; // LOOP na base instrumental armado (repete até a voz entrar)
  onLoopArmed?: (v: boolean) => void; // arma/desarma o LOOP (botão próprio ou FALAR)
  vocalCut?: boolean; // modo VOZ ligado — faixa local toca o instrumental
  onVocalCut?: (v: boolean) => void; // liga/desliga o modo VOZ (botão do transporte)
  // configuração de áudio das faixas locais — decide quais marcas o deck desenha
  mixCfg?: MixConfig;
}) {
  const [announcing, setAnnouncing] = useState(false);
  const [pickedHour, setPickedHour] = useState<number | null>(null);
  const activeDeck = useActiveDeck(snap);
  const [vizMode, cycleViz, setViz] = useVizMode();
  const isRadio = useMode() === "radio";

  const np = snap?.now_playing;
  const queue = snap?.queue ?? [];
  const hasNow = useMemo(() => !!np && !!np.track_id, [np]);

  // Barra do topo = a FILA INTEIRA (não uma música): total = faixa no ar + soma
  // de todas as próximas; "falta" = tempo até a playlist acabar. posição = quanto
  // já correu da fila (posição real da faixa atual, via SDK).
  const posNow = spotDeck?.posMs ?? 0;
  const durNow =
    spotDeck?.durMs && spotDeck.durMs > 0 ? spotDeck.durMs : np?.duration_ms ?? 0;
  // total = cabeça + resto. A cabeça (queue[0]) usa a duração REAL do deck ao vivo
  // (`durNow`, via SDK) quando toca; parado (`np` vazio, `durNow=0`) cai na duração
  // do próprio item — senão a 1ª faixa da fila sumia da conta e o total ficava 0 na
  // 1ª música adicionada (parecia "não recalcular").
  const queueTotalMs = useMemo(() => {
    if (queue.length === 0) return 0;
    const rest = queue
      .slice(1)
      .reduce((s, it) => s + (it.playable?.duration_ms ?? 0), 0);
    const headMs = durNow > 0 ? durNow : queue[0].playable?.duration_ms ?? 0;
    return headMs + rest;
  }, [durNow, queue]);
  const nn = nowNext(hasNow, np, queue);

  /* LOOP só faz sentido com faixa LOCAL no ar E vocal marcado: sem a marca não
     há o que segurar, e "loop até a voz entrar" numa faixa sem voz detectada não
     tem fim definido. Faixa de streaming não passa pelo `<audio>` daqui. */
  const canLoop = !!spotDeck?.live && (spotDeck?.vocalMs ?? 0) > 0;
  /* VOZ, como o LOOP, só existe com faixa LOCAL no ar — mas o gate segundo é
     OUTRO: em vez da marca do vocal, o INSTRUMENTAL já preparado por IA. Os dois
     estados apagados são distintos e o transporte explica cada um: "não há faixa
     minha no ar" passa sozinho; "falta preparar esta faixa" tem ação clara.
     `np.source` separa "local sem preparo" de "streaming" — o deck fica `live`
     nos três motores, mas só o local tem instrumental a tocar. */
  const npIsLocal = !!np && np.source !== "spotify" && np.source !== "youtube";
  const hasLocalLive = !!spotDeck?.live && npIsLocal;
  const canVocalCut = hasLocalLive && !!spotDeck?.instrumentalReady;
  /* PREPARO DA FAIXA NO AR — é o que transforma o botão apagado em ação.

     Instância própria do hook (a lista tem a dela). São dois polls só quando há
     trabalho em andamento, e nenhum quando não há — o custo é pequeno perto de
     erguer um store global só pra isto. */
  const stemWork = useStemWork();
  const liveTrackID = hasLocalLive ? (np?.track_id ?? "") : "";
  /* ⚠️ SÓ trabalho VIVO conta como ocupado. O endpoint devolve também `ready`
     (a FILA precisa disso para marcar as faixas já preparadas), e tratar qualquer
     valor como "em andamento" fazia a faixa PRONTA exibir ⏳ PROCESSANDO com o
     botão desabilitado — exatamente o contrário do certo. */
  const rawStem = stemWork.work[liveTrackID];
  const stemBusy: "queued" | "running" | null =
    rawStem === "queued" || rawStem === "running" ? rawStem : null;
  const prepareLive = () => {
    if (!liveTrackID) return;
    void prepareInstrumental(liveTrackID)
      .then((st) => {
        // 404 = ferramenta ausente nesta máquina: diz isso, em vez de silenciar.
        if (st === null) {
          toast.warning(
            "Este servidor não separa vozes — o recurso roda só no AutoDJ instalado no seu computador.",
          );
          return;
        }
        toast.info("Separando a voz — leva alguns minutos, roda em segundo plano.");
        stemWork.refresh(); // barra aparece no clique, não no próximo ciclo
      })
      .catch((e: Error) => toast.error(e.message));
  };
  /* O modo NÃO se desarma sozinho quando entra uma faixa sem preparo (um Spotify
     no meio, uma ainda não separada): ele vale pra toda faixa local seguinte, e
     desligar por baixo obrigaria o operador a religar a cada intervalo. O que muda
     é só o selo INSTRUMENTAL, que some — ali o original está tocando, e dizer o
     contrário seria mentira na tela.
     Mas o botão continua CLICÁVEL enquanto ligado (regra que mora no Transport):
     um botão aceso e desabilitado prenderia o operador no modo até uma faixa
     preparada voltar. */
  /* FALAR arma o LOOP junto e o solta ao encerrar — foi o pedido: a base segura
     enquanto se fala. O botão LOOP existe à parte porque FALAR só aparece no
     modo Rádio, e o padrão do app é Tocador. */
  const onTalk = () => {
    setAnnouncing((a) => {
      const next = !a;
      onLoopArmed?.(next && canLoop);
      return next;
    });
  };
  const wall = now
    ? `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}:${String(now.getSeconds()).padStart(2, "0")}`
    : "--:--:--";

  return (
    <>
      <NowPlayingHeader
        title={nn.now.full}
        next={nn.next.full}
        hasNext={nn.hasNext}
        now={now}
        posMs={posNow}
        durMs={durNow}
        queueMs={queueTotalMs}
        showTimers={isRadio}
        clockSteady={hasNow}
      />

      <NowViz
        mode={vizMode}
        onCycle={cycleViz}
        active={hasNow && connected}
        posMs={posNow}
        durMs={queueTotalMs}
      />
      <Timeline
        now={now}
        selectedHour={pickedHour}
        onPickHour={(h) => setPickedHour((p) => (p === h ? null : h))}
      />

      <div className="panel-slot" data-panel="fila">
        <ProgramList
          hasNow={hasNow}
          np={np}
          queue={queue}
          history={history}
          announcing={announcing}
          wall={wall}
          spotDeck={spotDeck}
          mix={mix}
          activeDeck={activeDeck}
          viz={vizMode}
          onPickViz={setViz}
          onSeek={onSpotSeek}
          onToggle={onSpotToggle}
          onEnqueue={onEnqueue}
          onEnqueueSpotify={onEnqueueSpotify}
          onEnqueueYouTube={onEnqueueYouTube}
          onDequeue={onDequeue}
          onMove={onMove}
          onPromote={onPromote}
          onPlayCued={onPlayCued}
          stopped={!wantPlay || !!spotDeck?.blocked}
          onPlayMaster={onPlayMaster}
          onStopMaster={onStopMaster}
          mixCfg={mixCfg}
          loopArmed={loopArmed}
          /* JÁ FILTRADO: "o modo está ligado E esta faixa tem instrumental" — é
             o que o selo INSTRUMENTAL do deck precisa saber. O deck não tem como
             decidir isso sozinho: o estado do preparo é lido ao vivo do endpoint
             pelo hook de playout, não vem no `Playable`. */
          vocalCut={vocalCut && canVocalCut}
        />
      </div>

      {/* Wrapper só pra ancorar os toasts: o host fica em `bottom: 100%` daqui,
          ou seja, colado ACIMA do transporte, largura inteira da coluna, sobrepondo
          o conteúdo em vez de empurrá-lo (sem layout shift no meio da operação). */}
      <div className="transport-wrap">
        <ToastHost />
        <Transport
          busy={busy}
          hasNext={nn.hasNext}
          hasPrev={(history?.length ?? 0) > 0}
          announcing={announcing}
          showTalk={isRadio}
          onNext={() => (onNext ? onNext() : post("/api/v1/playback/next"))}
          onPrev={() => onPrev?.()}
          onTalk={onTalk}
          loopArmed={loopArmed}
          canLoop={canLoop}
          onLoop={() => onLoopArmed?.(!loopArmed)}
          vocalCut={vocalCut}
          canVocalCut={canVocalCut}
          hasLocalLive={hasLocalLive}
          onVocalCut={() => onVocalCut?.(!vocalCut)}
          stemBusy={stemBusy}
          /* NÃO oferece a ação onde ela não existe (produção não tem a
             ferramenta): o botão volta a ser só informativo, em vez de um
             clique que só pode terminar em "não está instalado". */
          onPrepareStem={stemWork.available === false ? undefined : prepareLive}
          onPause={onSpotToggle}
          canPause={!!spotDeck?.live && !!onSpotToggle}
          paused={!!spotDeck?.live && !spotDeck.playing}
          stopped={!wantPlay || !!spotDeck?.blocked}
        />
      </div>

      {announcing && (
        <LiveAnnounceModal
          onClose={() => {
            setAnnouncing(false);
            onLoopArmed?.(false); // encerrou a locução ⇒ solta a base e segue pro vocal
          }}
        />
      )}
    </>
  );
}
