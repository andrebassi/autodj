"use client";

import type { VizMode } from "@/hooks/useVizMode";
import { DeckWave } from "@/components/DeckWave";
import { DeckMarks, type DeckMarksProps } from "@/components/DeckMarks";
import { TrackCover } from "@/components/TrackCover";
import { SourceBadge } from "@/components/SourceBadge";

/* Deck estilo VirtualDJ (Player A/B): badge + botão Play/Stop + faixa + barra de
   progresso REAL + timers. A = verde, B = azul (identidade fixa do deck). Qual
   deles está TOCANDO alterna por faixa (o ocioso carrega a próxima, cued).

   Visualização configurável (gear no par de decks): "line" = barra de progresso
   real; "wave" = waveform cosmético (Spotify é DRM, browser não lê PCM — a onda é
   ilustrativa, mas o preenchimento até o playhead segue a posição REAL). Ambos
   clicáveis p/ seek. Waveform real por PCM só p/ arquivos locais (Épico 2). */
export function Deck({
  deck,
  pos,
  trackId,
  artworkUrl,
  source,
  artist,
  title,
  elapsed,
  remaining,
  frac,
  cued,
  live,
  ytLive,
  ytCued,
  mixing,
  instrumental,
  marks,
  viz = "line",
  onSeek,
  onCtl,
  ctl = "play",
  ctlDisabled,
  onCoverClick,
  dropHot,
  onZoneDragOver,
  onZoneDragLeave,
  onZoneDrop,
}: {
  deck: "A" | "B";
  pos?: number; // posição GLOBAL na ordem (1=no ar, 2=cued) — pill sobre o badge A/B
  trackId?: string; // p/ derivar a capa (provider-agnóstico); "" = placeholder
  artworkUrl?: string; // capa direta (CDN Spotify) — bypassa o endpoint sessão-scoped
  source?: string; // motor da faixa → selo de origem (Spotify/YouTube) no deck
  artist: string;
  title: string;
  elapsed: string;
  remaining: string;
  frac: number; // 0..1 posição do playhead (real quando há feed)
  cued?: boolean; // deck parado no cue (próxima)
  live?: boolean; // há faixa carregada
  // faixa no ar é YouTube → o slot de capa vira o âncora (data-yt-slot) que o hook
  // useYouTubeOverlay usa p/ sobrepor o host do IFrame via position:fixed.
  ytLive?: boolean;
  // PRÓXIMA é YouTube → âncora data-yt-cue-slot p/ o 2º IFrame (pré-carga, mudo).
  ytCued?: boolean;
  // mixagem cross-motor em curso: "out" = este deck está saindo (descendo),
  // "in" = este deck já entrou tocando por cima (subindo). As duas músicas soam
  // juntas nesta janela — a tela precisa dizer isso, senão parece bug.
  mixing?: "in" | "out";
  // modo VOZ ligado E esta faixa está saindo sem o vocal AGORA. O selo existe pra
  // nunca ficar ambíguo o que está no ar: sem ele, "a música está estranha" e "o
  // modo está ligado" são indistinguíveis do lado do operador.
  instrumental?: boolean;
  // marcas de análise sobre a barra (trim, vocal, loop, ganho). Ausente em faixa
  // de streaming — não há PCM deste lado, logo não há nada medido pra desenhar.
  marks?: DeckMarksProps;
  viz?: VizMode; // linha de progresso ou waveform
  onSeek?: (frac: number) => void; // clicar/arrastar na barra → posiciona a música
  onCtl?: () => void; // botão Play/Stop do deck (antes da barra)
  ctl?: "play" | "stop"; // ícone/estado do botão
  ctlDisabled?: boolean; // sem ação possível (nada carregado / sem próxima)
  onCoverClick?: () => void; // clicar na capa → popup de detalhes da faixa
  // drop por-deck: soltar mídia/fila sobre ESTE deck. "cut" = deck ao vivo (corta
  // e toca agora, com popup); "cue" = deck ocioso (coloca como próxima, espera).
  dropHot?: false | "cut" | "cue"; // realce ao arrastar por cima (âmbar=cut, verde=cue)
  onZoneDragOver?: (e: React.DragEvent) => void;
  onZoneDragLeave?: (e: React.DragEvent) => void;
  onZoneDrop?: (e: React.DragEvent) => void;
}) {
  const seekable = !!live && !!onSeek;
  const pct = Math.max(0, Math.min(1, frac)) * 100;

  // divergência estrutural do espelho Spotify: o IFrame do YouTube exige um
  // elemento DOM visível. Um único host nunca-desmontado vive parqueado oculto em
  // page.tsx. NÃO movemos o host pra cá (mover um <iframe> no DOM o RECARREGA →
  // matava o playback, causa-raiz v0.2.4). Renderizamos só um ANCORA vazio 60×60
  // (`data-yt-slot`); o hook useYouTubeOverlay sobrepõe o host via position:fixed
  // casando o retângulo deste âncora — sem reparentar, sem recarregar o iframe.
  const showYtSlot = !!live && !!ytLive;
  // mesma mecânica p/ o deck OCIOSO: âncora do 2º IFrame, o engatilhado. A faixa
  // já fica carregada e muda aqui — na virada é só desmutar/tocar (v0.2.11).
  const showYtCueSlot = !showYtSlot && !!live && !!ytCued;

  // fração do clique dentro da barra (0..1), clampada.
  const fracFromEvent = (e: React.MouseEvent<HTMLDivElement>): number => {
    const r = e.currentTarget.getBoundingClientRect();
    if (r.width <= 0) return 0;
    return Math.max(0, Math.min(1, (e.clientX - r.left) / r.width));
  };
  const onProgClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!seekable) return;
    onSeek!(fracFromEvent(e));
  };
  // arraste (scrub): segura e move sobre a barra reposiciona continuamente.
  const onProgDrag = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!seekable || e.buttons !== 1) return;
    onSeek!(fracFromEvent(e));
  };

  return (
    <div
      onDragOver={onZoneDragOver}
      onDragLeave={onZoneDragLeave}
      onDrop={onZoneDrop}
      className={
        "deck deck-" +
        deck.toLowerCase() +
        (cued ? " cued" : "") +
        (live ? "" : " idle") +
        (mixing ? " mixing mixing-" + mixing : "") +
        (dropHot === "cut" ? " deck-hot-cut" : dropHot === "cue" ? " deck-hot-cue" : "")
      }
    >
      {pos ? <span className="deck-pos" aria-label={`Posição ${pos}`}>{pos}</span> : null}

      <span className="deck-badge">{deck}</span>

      <button
        type="button"
        className={"deck-ctl deck-ctl-" + ctl}
        onClick={onCtl}
        disabled={ctlDisabled || !onCtl}
        title={ctl === "stop" ? "Parar" : "Tocar"}
        aria-label={ctl === "stop" ? "Parar" : "Tocar"}
      >
        <span className={ctl === "stop" ? "deck-ico-stop" : "deck-ico-play"} />
      </button>

      {/* selo do motor (Spotify/YouTube) ancorado ABAIXO do badge+botão (canto
          inferior-esquerdo do deck), compacto — informa a origem sem roubar a
          coluna de título. Só ao vivo (title presente). */}
      {title ? <SourceBadge source={source} variant="deck" /> : null}

      <div className="deck-body">
        <div className="deck-meta">
          {showYtSlot ? (
            // mesmo box 60×60 da capa (só chaveia o motor, não redimensiona): o
            // host do IFrame do YouTube é movido pra cá enquanto a faixa tocar.
            <div
              data-yt-slot
              className="track-cover cover-deck yt-slot"
              aria-label="Vídeo do YouTube no ar"
            >
              {/* Fallback SOB o iframe: quando o embed falha (dono desabilitou a
                  incorporação → tela PRETA de erro do próprio YouTube), o hook
                  useYouTubeOverlay esconde o host do iframe e ESTA capa (thumbnail
                  real da faixa) aparece no lugar — nunca mais a caixa preta no
                  slot. Com o embed OK, o iframe (z-index maior) cobre isto. */}
              <TrackCover
                trackId={trackId}
                src={artworkUrl}
                size="deck"
                alt={title || artist}
              />
            </div>
          ) : showYtCueSlot ? (
            // deck engatilhado: mesmo box 60×60, âncora do 2º IFrame (pré-carga).
            // O vídeo já está carregado e MUDO aqui — a virada não baixa nada.
            <div
              data-yt-cue-slot
              className="track-cover cover-deck yt-slot"
              aria-label="Vídeo do YouTube pré-carregado (próxima)"
            >
              <TrackCover
                trackId={trackId}
                src={artworkUrl}
                size="deck"
                alt={title || artist}
              />
            </div>
          ) : (
            <TrackCover
              trackId={trackId}
              src={artworkUrl}
              size="deck"
              alt={title || artist}
              onClick={onCoverClick}
            />
          )}
          <div className="deck-meta-txt">
            {/* layout Spotify: título forte em cima, intérprete cinza embaixo.
                Sem título (ocioso) → mostra só a linha de status (artist). */}
            {title ? (
              <>
                <span className="deck-title">{title}</span>
                <span className="deck-artist">{artist}</span>
              </>
            ) : (
              <span className="deck-artist deck-artist-idle">{artist}</span>
            )}
          </div>
          {/* Selo do modo VOZ. Vem ANTES do MIX/CUE de propósito: com os dois na
              tela (mixando com o modo ligado) o `margin-left: auto` empurra este
              pro fim da linha e o outro encosta nele, em vez de brigarem pela
              mesma âncora. */}
          {instrumental && (
            <span
              className="deck-instrumental"
              title="SEM-VOZ ligado — esta faixa está no ar sem a voz"
            >
              SEM-VOZ
            </span>
          )}
          {mixing ? (
            <span
              className={"deck-mix deck-mix-" + mixing}
              title={
                mixing === "in"
                  ? "Entrando na mixagem (subindo)"
                  : "Saindo na mixagem (descendo)"
              }
            >
              MIX {mixing === "in" ? "▲" : "▼"}
            </span>
          ) : (
            cued && <span className="deck-cue">CUE</span>
          )}
        </div>

        {/* As marcas são IRMÃS da barra, não filhas: a `.deck-prog` tem
            `overflow: hidden` (é o que arredonda o preenchimento), então o tick
            do vocal e o selo de ganho seriam cortados lá dentro. O invólucro
            `.deck-viz` carrega o `position: relative` e a margem que antes eram
            da barra, e as marcas ficam por cima sem clipe. */}
        <div className="deck-viz">
          {viz === "wave" ? (
            <DeckWave
              deck={deck}
              frac={frac}
              live={!!live}
              seekable={seekable}
              onDown={onProgClick}
              onMove={onProgDrag}
            />
          ) : (
            <div
              className={"deck-prog" + (seekable ? " seekable" : "")}
              onMouseDown={onProgClick}
              onMouseMove={onProgDrag}
              title={seekable ? "Clique/arraste p/ avançar ou retroceder" : undefined}
            >
              <span className="deck-prog-fill" style={{ width: `${pct}%` }} />
              {live && frac > 0 && (
                <span className="deck-prog-head" style={{ left: `${pct}%` }} />
              )}
            </div>
          )}
          {live && marks ? <DeckMarks {...marks} /> : null}
        </div>
      </div>

      <div className="deck-timers">
        <span className="deck-elapsed">{elapsed}</span>
        <span className="deck-remain">{remaining}</span>
      </div>
    </div>
  );
}
