import { useState } from "react";
import type { Playable, QueueItem } from "@/lib/types";
import type { CrossMixState } from "@/hooks/useCrossEngineMix";
import type { VizMode } from "@/hooks/useVizMode";
import type { DeckMarksProps } from "@/components/DeckMarks";
import { Deck } from "@/components/Deck";
import { VizMenu } from "@/components/VizMenu";
import { nowNext } from "@/lib/nowNext";
import { playableToMedia, type OpMedia } from "@/lib/operational";
import { trackGain, targetLUFSFromVol } from "@/lib/gain";
import type { MixConfig } from "@/lib/uiContent";

/* Par de decks A/B (VirtualDJ). A = verde, B = azul (identidade fixa). Qual deles
   está TOCANDO alterna por faixa (`activeDeck`): o deck ativo mostra o now-playing
   (timers reais + botão Parar), o ocioso mostra a próxima engatilhada (CUE + botão
   Tocar). Não empurra tudo sempre p/ o A.

   Quando o now-playing é Spotify dirigido pelo SDK (`liveActive`), o deck ativo usa
   a posição REAL da música (barra de progresso segue de verdade + seek). Sem feed
   real → barra vazia (não inventa posição). */
function fmt(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

type RoleProps = {
  pos?: number; // posição ABSOLUTA na sequência (base=tocadas): no ar=base+1, cued=base+2
  trackId: string;
  artworkUrl: string;
  source: string; // motor da faixa → selo de origem (Spotify/YouTube) no deck
  artist: string;
  title: string;
  elapsed: string;
  remaining: string;
  frac: number;
  live: boolean;
  cued: boolean;
  // faixa no ar é YouTube → o slot de capa 60×60 deste deck vira o alvo do IFrame
  // que está no ar (o host é sobreposto por CSS, nunca reparentado).
  ytLive?: boolean;
  // a PRÓXIMA é YouTube → este deck (ocioso) vira o alvo do 2º IFrame, o
  // engatilhado da pré-carga (v0.2.11): o vídeo já fica carregado e mudo aqui,
  // pronto p/ entrar no ar sem buffering.
  ytCued?: boolean;
  // janela da mixagem cross-motor: "out" = deck que está saindo (descendo),
  // "in" = deck que já entrou tocando por cima (subindo).
  mixing?: "in" | "out";
  // modo VOZ atuando nesta faixa agora (selo INSTRUMENTAL).
  instrumental?: boolean;
  marks?: DeckMarksProps;
  onSeek?: (frac: number) => void;
  onCtl?: () => void;
  ctl: "play" | "stop";
  ctlDisabled: boolean;
  onCoverClick?: () => void;
};

/* Marcas de análise de um Playable. Devolve `undefined` quando não há NADA a
   desenhar — faixa de streaming (tudo zero por construção), faixa ainda não
   analisada, ou config de áudio ainda carregando. Melhor não desenhar do que
   desenhar uma marca que não corresponde ao que o playout está fazendo. */
function marksFor(
  p: { cue_in_ms?: number; cue_out_ms?: number; vocal_ms?: number; loudness_lufs?: number; category?: string; duration_ms?: number } | undefined,
  durMs: number,
  mix: MixConfig | undefined,
  loopArmed: boolean,
): DeckMarksProps | undefined {
  if (!p || !mix || durMs <= 0) return undefined;
  // trim é por TIPO de mídia: música e vinheta têm interruptores separados.
  const trimOn = (p.category ?? "") === "jingle" ? mix.trimVinhetas : mix.trimMusicas;
  const cueInMs = trimOn ? (p.cue_in_ms ?? 0) : 0;
  const cueOutMs = trimOn ? (p.cue_out_ms ?? 0) : 0;
  const vocalMs = p.vocal_ms ?? 0;
  const gain = trackGain({
    loudnessLUFS: p.loudness_lufs ?? 0,
    targetLUFS: targetLUFSFromVol(mix.normalizeVol),
    normalize: mix.normalize,
  });
  if (!cueInMs && !cueOutMs && !vocalMs && gain === 1) return undefined;
  return { durMs, cueInMs, cueOutMs, vocalMs, loopArmed, gain, normalize: mix.normalize };
}

export function DeckPair({
  hasNow,
  np,
  queue,
  posBase = 0,
  livePosMs,
  liveDurMs,
  liveActive = false,
  mix,
  viz = "line",
  onPickViz,
  onSeek,
  onToggle,
  playing = false,
  stopped = false,
  onPlayMaster,
  onStopMaster,
  activeDeck = "A",
  canDrop = false,
  onDeckDrop,
  onPlayCued,
  onOpenDetail,
  mixCfg,
  loopArmed = false,
  vocalCut = false,
}: {
  hasNow: boolean;
  np: Playable | undefined;
  queue: QueueItem[];
  posBase?: number; // nº de faixas já tocadas — deslocamento p/ a numeração absoluta
  livePosMs?: number; // posição real (ms) — Spotify via SDK
  liveDurMs?: number; // duração real (ms) — Spotify via SDK
  liveActive?: boolean; // há feed real dirigindo o deck ativo
  // mixagem cross-motor em curso (Spotify↔YouTube): a PRÓXIMA já está tocando por
  // cima da que sai, mas o now-playing do servidor só muda no fim natural. Sem
  // isto o deck ocioso ficava parado em CUE/00:00 com a música dele no ar.
  mix?: CrossMixState;
  viz?: VizMode; // linha de progresso ou waveform (cosmético)
  onPickViz?: (m: VizMode) => void; // gear discreto troca a visualização
  onSeek?: (frac: number) => void; // seek na barra do deck ativo (Spotify)
  onToggle?: () => void; // play/pause do deck ativo (now-playing)
  playing?: boolean; // deck ativo tocando? (ícone do botão)
  stopped?: boolean; // estação parada (gate mestre off) — deck ao vivo vira ▶ PLAY
  onPlayMaster?: () => void; // ▶ do deck ao vivo (parado) → estação no ar (PLAY mestre)
  onStopMaster?: () => void; // ⏹ do deck ao vivo (no ar) → estação fora do ar (STOP mestre)
  activeDeck?: "A" | "B"; // qual deck físico está tocando agora
  canDrop?: boolean; // aceita soltar mídia/fila sobre os decks
  // drop sobre UM deck. `deckIsLive` = soltou no deck que está TOCANDO agora
  // (corta + toca já, com popup). false = deck ocioso / nada tocando (coloca
  // como próxima e espera, sem popup).
  onDeckDrop?: (e: React.DragEvent, deckIsLive: boolean) => void;
  // ▶ do deck OCIOSO (cued): promove a próxima já carregada para o ar agora.
  // Sem isso o botão fica desabilitado.
  onPlayCued?: () => void;
  // clicar na capa de um deck → popup de detalhes da faixa daquele deck.
  onOpenDetail?: (media: OpMedia) => void;
  // configuração de áudio das faixas locais — decide QUAIS marcas valem (o trim
  // desligado não pode desenhar zona de corte que não vai acontecer).
  mixCfg?: MixConfig;
  loopArmed?: boolean; // LOOP segurando a base agora (pulsa a faixa do loop)
  vocalCut?: boolean; // modo VOZ atuando de fato (instrumental no ar)
}) {
  const { now, next, hasNext, live } = nowNext(hasNow, np, queue);
  // qual deck físico está de fato AO VIVO agora: o ativo, e só se estiver tocando.
  // Nada tocando ⇒ nenhum deck é "cut" — todo drop vira "cue" (coloca e espera).
  const liveA = playing && activeDeck === "A";
  const liveB = playing && activeDeck === "B";
  // realce por-deck ao arrastar por cima. Aceita mídia (local/Spotify) e itens da
  // própria fila (reorder → adiantar). "cut" no deck ao vivo, "cue" no ocioso.
  const [hot, setHot] = useState<null | "A" | "B">(null);
  const isDropDrag = (e: React.DragEvent) => {
    const t = e.dataTransfer.types;
    return (
      t.includes("text/autodj-track") ||
      t.includes("text/autodj-spotify-uri") ||
      t.includes("text/autodj-youtube-uri") ||
      t.includes("text/autodj-reorder")
    );
  };
  const overFor = (deckId: "A" | "B") => (e: React.DragEvent) => {
    if (!canDrop || !onDeckDrop || !isDropDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    // reorder (item da fila) viaja como effectAllowed="move" — o dropEffect PRECISA
    // ser "move" senão o browser recusa o drop. Mídia (copy) fica "copy".
    e.dataTransfer.dropEffect = e.dataTransfer.types.includes("text/autodj-reorder")
      ? "move"
      : "copy";
    setHot(deckId);
  };
  const dropFor = (deckId: "A" | "B") => (e: React.DragEvent) => {
    setHot(null);
    if (!onDeckDrop || !isDropDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    onDeckDrop(e, deckId === "A" ? liveA : liveB);
  };
  const hotFor = (deckId: "A" | "B"): false | "cut" | "cue" => {
    if (hot !== deckId) return false;
    return (deckId === "A" ? liveA : liveB) ? "cut" : "cue";
  };

  // duração/posição do deck que toca: real quando o SDK dirige, senão sem posição.
  const durNow = liveActive && liveDurMs && liveDurMs > 0 ? liveDurMs : now.durMs;
  const posNow = liveActive ? Math.min(livePosMs ?? 0, durNow) : 0;
  const nowFrac = liveActive && durNow > 0 ? posNow / durNow : 0;

  // JANELA DA MIXAGEM: as duas faixas estão no ar ao mesmo tempo. Só vale se a
  // entrante da janela é mesmo a que a UI mostra como próxima (mudou a fila no
  // meio → cai fora e a tela volta ao normal). A uri mora no `Playable` (o
  // `DeckInfo` do nowNext só carrega o que a tela desenha), então casa direto
  // contra `queue[1]` — a mesma fonte que o hook usou p/ abrir a janela.
  const mixing = !!mix?.active && hasNext && mix.uri === (queue[1]?.playable.uri ?? "");
  // tempo da entrante: ela começou do zero quando a janela abriu.
  const mixPos = mixing
    ? next.durMs > 0
      ? Math.min(mix!.elapsedMs, next.durMs)
      : mix!.elapsedMs
    : 0;
  const mixFrac = mixing && next.durMs > 0 ? mixPos / next.durMs : 0;

  // papel "tocando agora" (vai pro deck ativo). Posição ABSOLUTA na sequência:
  // base+1 (base = nº de tocadas), então cued = base+2 e a cauda começa em base+3.
  // Só numera se há faixa no ar.
  //
  // O botão do deck ao vivo virou o PLAY/STOP MESTRE da estação (os botões de baixo
  // saíram): parado com cabeça carregada → ▶ arranca (o gesto do usuário destrava o
  // autoplay do browser p/ Spotify/local — por isso "AUTO On" sozinho não soava). No
  // ar → ⏹ tira do ar (para os dois motores). Sempre habilitado quando há cabeça.
  const nowRole: RoleProps = {
    pos: live ? posBase + 1 : undefined,
    trackId: live ? now.trackId : "",
    artworkUrl: live ? now.artworkUrl : "",
    source: live ? now.source : "",
    artist: live ? now.artist : "—",
    title: now.title,
    elapsed: fmt(posNow),
    remaining: "-" + fmt(Math.max(0, durNow - posNow)),
    frac: nowFrac,
    live,
    cued: false,
    ytLive: live && np?.source === "youtube",
    mixing: mixing ? "out" : undefined,
    /* O selo é sobre o que ESTÁ saindo, não sobre o que o operador pediu.

       `vocalCut` já chega filtrado pelo PlayerScreen ("modo ligado E esta faixa
       tem instrumental preparado"), e é a única régua desde a 0.2.32: com o
       cancelamento de centro aposentado, INSTRUMENTAL no deck significa
       exatamente uma coisa — a fonte no ar é o arquivo separado pela IA. */
    instrumental: live && vocalCut,
    marks: live ? marksFor(np, durNow, mixCfg, loopArmed) : undefined,
    onSeek: liveActive ? onSeek : undefined,
    onCtl: stopped ? (live ? onPlayMaster : undefined) : onStopMaster,
    ctl: stopped ? "play" : "stop",
    ctlDisabled: stopped ? !live || !onPlayMaster : !onStopMaster,
    onCoverClick:
      live && np && onOpenDetail
        ? () => onOpenDetail(playableToMedia(np))
        : undefined,
  };

  // papel "próxima engatilhada" (vai pro deck ocioso). O ▶ do deck cued promove a
  // próxima JÁ CARREGADA para o ar agora (é exatamente a que a UI mostra, não a
  // próxima-da-fila — o backend mantém engine e mirror em lockstep). Habilitado só
  // quando há próxima e o handler existe; senão fica cinza (`onCtl: undefined`).
  const nextRole: RoleProps = {
    pos: hasNext ? posBase + 2 : undefined,
    trackId: hasNext ? next.trackId : "",
    artworkUrl: hasNext ? next.artworkUrl : "",
    source: hasNext ? next.source : "",
    artist: next.artist,
    title: next.title,
    // durante a mixagem a entrante JÁ toca: tempo correndo e barra andando, no
    // lugar do 00:00 congelado do cue.
    elapsed: mixing ? fmt(mixPos) : "00:00",
    // engatilhada mostra a duração cheia; tocando na mixagem mostra o que falta.
    remaining: mixing ? "-" + fmt(Math.max(0, next.durMs - mixPos)) : fmt(next.durMs),
    frac: mixFrac,
    live: hasNext,
    cued: hasNext && !mixing, // tocando por cima ⇒ não é mais "CUE", é "MIX"
    mixing: mixing ? "in" : undefined,
    /* Na janela da mixagem a entrante JÁ está soando. Mas o selo dela não pode
       ser adivinhado: o preparo da PRÓXIMA é lido ao vivo do endpoint pelo hook
       de playout, que só consulta a faixa NO AR. Sem esse dado, anunciar
       INSTRUMENTAL aqui seria chute — e ela pode muito bem entrar com voz. */
    instrumental: false,
    // o deck engatilhado mostra as marcas DELE: o operador vê o que vem antes de
    // entrar no ar. O LOOP nunca pulsa aqui (só a faixa no ar pode estar em loop).
    marks: hasNext
      ? marksFor(queue[1]?.playable, next.durMs, mixCfg, false)
      : undefined,
    ytLive: false, // o IFrame no ar mora sempre no deck ao vivo
    // pré-carga: se a próxima é YouTube, este deck hospeda o 2º IFrame (mudo,
    // já bufferizado) — é o que torna a virada instantânea.
    ytCued: hasNext && next.source === "youtube",
    onSeek: undefined,
    onCtl: hasNext ? onPlayCued : undefined,
    ctl: "play",
    ctlDisabled: !hasNext || !onPlayCued,
    onCoverClick:
      hasNext && queue[1] && onOpenDetail
        ? () => onOpenDetail(playableToMedia(queue[1].playable))
        : undefined,
  };

  const aRole = activeDeck === "A" ? nowRole : nextRole;
  const bRole = activeDeck === "B" ? nowRole : nextRole;

  return (
    <li className="deckpair">
      {onPickViz && <VizMenu mode={viz} onPick={onPickViz} />}
      <Deck
        deck="A"
        viz={viz}
        {...aRole}
        dropHot={hotFor("A")}
        onZoneDragOver={overFor("A")}
        onZoneDragLeave={() => setHot((h) => (h === "A" ? null : h))}
        onZoneDrop={dropFor("A")}
      />
      <Deck
        deck="B"
        viz={viz}
        {...bRole}
        dropHot={hotFor("B")}
        onZoneDragOver={overFor("B")}
        onZoneDragLeave={() => setHot((h) => (h === "B" ? null : h))}
        onZoneDrop={dropFor("B")}
      />
    </li>
  );
}
