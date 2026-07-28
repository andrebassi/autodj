"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import type { NowPlaying } from "@/lib/types";
import { fadeEnabled, fadeMs } from "@/lib/fade";
import { resumeFor, saveResume } from "@/lib/resume";
import { useYouTubePlayer } from "@/hooks/useYouTubePlayer";

/* A retomada pós-refresh mora em `lib/resume.ts` desde a 0.2.26 — UM registro pros
   três motores. Este hook tinha a sua cópia, com CHAVE PRÓPRIA
   (`autodj.ytplayout.v1`), e o Spotify tinha outra: dois estados paralelos pra uma
   coisa que só pode ter uma faixa no ar, e o motor local não tinha nenhuma. */

// fade de transição (PRÓXIMA/RETROCEDER): faixa nova entra subindo e a que sai
// desce. A DURAÇÃO vem do ⚙ Modo do Player (`fadeMs()`: 0/3/5/7s) e é lida NO
// MOMENTO do fade — antes era uma constante 3000 aqui, que ignorava a escolha do
// operador. YT_TARGET = ganho pleno (0..1 no player, →0..100 no IFrame).
const YT_TARGET = 1.0;

/** papel corrente de um host de IFrame: no ar, engatilhado (pré-carga) ou nenhum
 *  (parqueado fora da tela). O overlay usa isso p/ escolher o slot-alvo. */
export type YouTubeHostRole = "live" | "cue" | null;

export type YouTubeDeck = {
  /** now-playing é YouTube e está sendo dirigido pelo IFrame do browser */
  isYouTube: boolean;
  /** há feed REAL dirigindo a posição do deck (generaliza `isYouTube` p/ paridade
   *  com PlayoutDeck.live do Spotify/local). */
  live: boolean;
  /** IFrame realmente tocando (não pausado) */
  playing: boolean;
  /** posição ao vivo (ms) — dirige o waveform/timers do deck */
  posMs: number;
  /** duração real reportada pelo IFrame (ms) */
  durMs: number;
  /** tocando MUDO por bloqueio de autoplay (pós-refresh): a estação quer tocar,
   *  mas som só depois de um gesto. A UI vira o botão do deck em ▶ — deixá-lo em
   *  ⏹ mentiria "no ar", e o clique de quem quer som pararia a estação. */
  blocked?: boolean;
};

/* Driver de playout do YouTube no nível da página (Story 6.7), mirror do
   useSpotifyPlayout. A fila é a agenda; quando o now-playing é uma faixa YouTube,
   ela toca *out-of-band* pelo IFrame Player API (embed/DRM, sem PCM — nunca no
   bus de mix). Este hook faz a ponte entre o now-playing do servidor (WebSocket)
   e o player IFrame do browser:

   1. now_playing.source === "youtube" e a URI muda → play(uri);
   2. lê a posição do IFrame (poll no player) e interpola entre leituras;
   3. detecta o fim (ENDED) e chama POST /playback/next — a fila avança sozinha
      (hard_cut), + fallback por duração (o ENDED às vezes não chega).

   Sem gate de premium: o IFrame toca qualquer vídeo embeddable. O gate de login
   (espelho fiel do Spotify) vive na aba/import, não aqui.

   PRÉ-CARGA / DOIS PLAYERS (v0.2.11): o hook monta DUAS instâncias do IFrame,
   uma por host. `liveIdx` diz qual está no ar; a outra fica engatilhada com
   queue[1] já bufferizada e muda. Quando o servidor avança a fila, se a
   engatilhada já tem a URI que entrou no ar, é só standby()+goLive()+trocar o
   índice — troca instantânea, sem loadVideoById nem buffering no meio. Se não
   tiver (fila mudou, faixa fora de ordem), cai no caminho antigo `play(uri)` no
   player que já está no ar. Nunca regride: pior caso == comportamento ≤v0.2.10.

   Os papéis são dos PLAYERS, não dos decks A/B: o ping-pong visual do deck é do
   PlayerScreen (useActiveDeck) e segue independente — o overlay do iframe é
   ancorado por slot (`[data-yt-slot]` no ar, `[data-yt-cue-slot]` engatilhado),
   então este hook não precisa saber qual letra está no ar. */
export function useYouTubePlayout(
  hostARef: RefObject<HTMLDivElement | null>,
  hostBRef: RefObject<HTMLDivElement | null>,
  snap: NowPlaying | null,
  post: (path: string) => void,
  wantPlay = true,
): {
  deck: YouTubeDeck;
  toggle: () => void;
  seek: (frac: number) => void;
  fadeOut: () => Promise<void>;
  /** põe `uri` no ar já subindo de 0 (mixagem por cima do outro motor) */
  startMix: (uri: string) => boolean;
  /** papel de cada host (índice = A,B) p/ o overlay mirar o slot certo */
  roles: [YouTubeHostRole, YouTubeHostRole];
} {
  // qual instância está NO AR. Espelhado em ref porque o ramp de volume e o
  // handoff rodam fora do ciclo de render e precisam do valor no instante exato.
  const [liveIdx, setLiveIdx] = useState<0 | 1>(0);
  const liveIdxRef = useRef<0 | 1>(0);
  liveIdxRef.current = liveIdx;

  const p0 = useYouTubePlayer(hostARef, liveIdx === 0);
  const p1 = useYouTubePlayer(hostBRef, liveIdx === 1);
  const players = useMemo(() => [p0, p1] as const, [p0, p1]);
  const playersRef = useRef(players);
  playersRef.current = players;
  const live = players[liveIdx];
  const cued = players[liveIdx === 0 ? 1 : 0];
  const status = live.status;

  // comandos SEMPRE resolvidos pela ref → acertam a instância que está no ar no
  // momento da chamada (e não a capturada no render em que o callback nasceu).
  const play = useCallback((uri: string, startMs = 0) => {
    playersRef.current[liveIdxRef.current].play(uri, startMs);
  }, []);
  const ytToggle = useCallback(() => {
    playersRef.current[liveIdxRef.current].toggle();
  }, []);
  const ytStop = useCallback(() => {
    playersRef.current[liveIdxRef.current].stop();
  }, []);
  const ytSeek = useCallback((ms: number) => {
    playersRef.current[liveIdxRef.current].seek(ms);
  }, []);
  const setVolume = useCallback((v: number) => {
    playersRef.current[liveIdxRef.current].setVolume(v);
  }, []);

  // ramp de volume do IFrame (fade). Cancela ramp em voo. ytVol espelha o último
  // ganho aplicado (base do próximo ramp). Passos de 100ms.
  const ytVol = useRef(YT_TARGET);
  const fadeTimer = useRef<number | null>(null);
  const clearFade = useCallback(() => {
    if (fadeTimer.current !== null) {
      clearInterval(fadeTimer.current);
      fadeTimer.current = null;
    }
  }, []);
  const rampVol = useCallback(
    (to: number, ms: number): Promise<void> =>
      new Promise((resolve) => {
        clearFade();
        const from = ytVol.current;
        if (ms <= 0 || Math.abs(to - from) < 0.01) {
          ytVol.current = to;
          setVolume(to);
          resolve();
          return;
        }
        const steps = Math.max(1, Math.round(ms / 100));
        let i = 0;
        fadeTimer.current = window.setInterval(() => {
          i++;
          const v = Math.max(0, Math.min(1, from + (to - from) * (i / steps)));
          ytVol.current = v;
          setVolume(v);
          if (i >= steps) {
            clearFade();
            ytVol.current = to;
            setVolume(to);
            resolve();
          }
        }, 100);
      }),
    [clearFade, setVolume],
  );
  const fadeIn = useCallback(async () => {
    if (!fadeEnabled()) {
      ytVol.current = YT_TARGET;
      setVolume(YT_TARGET);
      return;
    }
    ytVol.current = 0;
    setVolume(0);
    await rampVol(YT_TARGET, fadeMs());
  }, [setVolume, rampVol]);
  const fadeOut = useCallback(async () => {
    if (!status.playing || !fadeEnabled()) return;
    await rampVol(0, fadeMs());
  }, [status.playing, rampVol]);
  useEffect(() => () => clearFade(), [clearFade]);

  const np = snap?.now_playing;
  const npIsYouTube = !!np && np.source === "youtube" && !!np.uri && !!np.track_id;
  const npUri = npIsYouTube ? np!.uri : "";
  const npDurMs = np?.duration_ms ?? 0;
  // faixas atrás na fila. Guarda-vazio: avançar com a fila vazia ESVAZIA o deck
  // (o backend limpa o now_playing). Só pulamos automaticamente quando há p/ onde
  // ir — senão a faixa (mesmo bloqueada) fica visível no deck em vez de sumir.
  const queueLen = snap?.queue?.length ?? 0;

  // PAUSE do deck = intent de segurar a faixa (distinto do wantPlay mestre).
  // Congela o auto-avanço. Reseta a cada troca de faixa.
  const [userPaused, setUserPaused] = useState(false);
  useEffect(() => {
    setUserPaused(false);
  }, [npUri]);

  // a faixa ATUAL já produziu som? Reset a cada troca. Usado pelo guarda-vazio:
  // não sumir com o deck ao "pular" uma faixa que nunca chegou a tocar (bloqueio
  // de embed / erro) quando não há próxima na fila.
  const playedThisUri = useRef(false);
  useEffect(() => {
    playedThisUri.current = false;
  }, [npUri]);
  useEffect(() => {
    if (npIsYouTube && status.playing) playedThisUri.current = true;
  }, [npIsYouTube, status.playing]);


  // próxima da fila (queue[0] = no ar, queue[1] = próxima). É ela que vai pro
  // player engatilhado — o deck ocioso deixa de mostrar só metadado e passa a ter
  // o áudio pronto.
  const nextPlayable = snap?.queue?.[1]?.playable;
  const nextIsYouTube =
    !!nextPlayable && nextPlayable.source === "youtube" && !!nextPlayable.uri;
  const nextUri = nextIsYouTube ? nextPlayable!.uri : "";

  // MIXAGEM CROSS-MOTOR: faixa que já foi posta no ar AQUI *antes* de virar o
  // now-playing (começou por cima do fim da faixa do outro motor). Ref p/ os
  // effects/callbacks; state espelhado porque `roles` (overlay) e a pré-carga
  // precisam saber que a instância engatilhada virou som de verdade.
  const preStarted = useRef<string>("");
  const [mixUri, setMixUri] = useState("");
  const armMix = useCallback((uri: string) => {
    preStarted.current = uri;
    setMixUri(uri);
  }, []);
  const disarmMix = useCallback(() => {
    preStarted.current = "";
    setMixUri("");
  }, []);

  // 1) now-playing YouTube mudou → carrega no IFrame. lastPlayed evita re-disparo
  //    a cada snapshot do WS.
  const lastPlayed = useRef<string>("");
  useEffect(() => {
    // Cuea a faixa assim que ela entra no ar (loadVideoById). Se o autoplay for
    // bloqueado (sem gesto ainda), o vídeo fica CUED (-1/5) e o 1º gesto do
    // usuário — capturado pelo listener em useYouTubePlayer — dá o playVideo()
    // dentro da janela de ativação, tocando com som. NÃO gatear aqui em "unlocked":
    // o cue precisa acontecer cedo p/ o gesto ter o que iniciar.
    if (!npIsYouTube || !status.ready || !wantPlay) return;
    if (npUri === lastPlayed.current) return;
    // esta faixa JÁ está no ar aqui (pré-início da mixagem cross-motor) — o
    // servidor só agora avançou a fila. Adota o trilho sem recarregar o vídeo: o
    // player já está tocando há `fadeMs()` e recarregar voltaria pro 0.
    if (npUri && npUri === preStarted.current) {
      disarmMix();
      lastPlayed.current = npUri;
      return;
    }
    lastPlayed.current = npUri;
    const resumeMs = resumeFor(npUri, npDurMs);

    // HANDOFF: a faixa que entrou no ar já está engatilhada e bufferizada no
    // outro player → troca de papel em vez de recarregar. Só quando não há
    // posição a restaurar (resume manda no player que já está no ar).
    const idx = liveIdxRef.current;
    const other = playersRef.current[idx === 0 ? 1 : 0];
    if (resumeMs <= 1500 && other.status.ready && other.status.currentUri === npUri) {
      playersRef.current[idx].standby();
      const nidx: 0 | 1 = idx === 0 ? 1 : 0;
      // ref ANTES do setState: rampVol/fadeIn logo abaixo já têm de mirar o novo
      // player (o re-render com o liveIdx novo vem depois).
      liveIdxRef.current = nidx;
      setLiveIdx(nidx);
      other.goLive();
      void fadeIn();
      return;
    }

    void (async () => {
      // A POSIÇÃO VAI NO PRÓPRIO LOAD (`startSeconds`), não num seek depois.
      // Antes: `loadVideoById(id)` + `seekTo` 700ms depois — e o seek chegava com o
      // vídeo ainda carregando (UNSTARTED/BUFFERING), onde a IFrame API
      // simplesmente descarta o pedido. Resultado: o refresh recomeçava do 00:00
      // apesar do resume estar salvo e correto.
      play(npUri, resumeMs);
      if (resumeMs > 1500) {
        // retomada não é faixa nova: ganho pleno direto, sem fade-in.
        ytVol.current = YT_TARGET;
        setVolume(YT_TARGET);
        base.current = { pos: resumeMs, at: Date.now(), playing: true, uri: npUri };
        setLivePos(resumeMs);
      } else {
        void fadeIn();
      }
    })();
  }, [
    npIsYouTube,
    npUri,
    status.ready,
    wantPlay,
    play,
    ytSeek,
    setVolume,
    fadeIn,
    disarmMix,
  ]);

  // PLAY mestre desligado → pausa o IFrame e re-arma o trilho.
  useEffect(() => {
    if (wantPlay) return;
    ytStop();
    lastPlayed.current = "";
  }, [wantPlay, ytStop]);

  // now-playing deixou de ser YouTube → solta o trilho.
  useEffect(() => {
    if (!npIsYouTube) lastPlayed.current = "";
  }, [npIsYouTube]);

  // ABORTO DA MIXAGEM: a faixa pré-iniciada TEM de virar o now-playing logo em
  // seguida. Se entrou OUTRA no ar (operador mexeu na fila / tocar-agora no meio
  // da mixagem), o pré-início vira música fantasma por cima da que entrou —
  // pausa e solta. Se a que entrou for YouTube, o effect 1 já a carrega por cima.
  const npAnyUri = snap?.now_playing?.uri ?? "";
  useEffect(() => {
    const pre = preStarted.current;
    if (!pre || !npAnyUri || npAnyUri === pre) return;
    disarmMix();
    if (!npIsYouTube) ytStop();
  }, [npAnyUri, npIsYouTube, ytStop, disarmMix]);

  // 1b) PRÉ-CARGA da próxima: engatilha queue[1] no player que NÃO está no ar,
  //     mudo e já bufferizado. É o que tira o buraco de silêncio da virada — sem
  //     isto o áudio só começava a ser baixado quando a faixa entrava no ar.
  //     Se a próxima não for YouTube (ou não houver), libera a instância.
  //
  //     A INSTÂNCIA ALVO SAI DA **REF** (`liveIdxRef`), NUNCA do `cued` do render
  //     (bug da virada YouTube→YouTube, 0.2.15). No handoff o effect 1 troca os
  //     papéis DENTRO do próprio effect: `liveIdxRef` já aponta pro player que
  //     entrou no ar, mas o state `liveIdx` (e portanto `cued`) só muda no render
  //     seguinte. Como `npUri` acabou de mudar, ESTE effect roda no MESMO flush,
  //     logo depois — e com o `cued` velho ele mirava justamente o player que
  //     acabara de receber `goLive()`: `cue()` (mute + cueVideoById da próxima) ou
  //     `standby()` (pause + mute) matavam o som um instante depois de começar.
  //     Sintoma: deck da entrante carregado, parado em 00:00, e um PLAY/RETOMAR
  //     (ou refresh) fazia tocar. Resolvendo pela ref, o alvo é sempre o player
  //     realmente ocioso.
  useEffect(() => {
    const idle = playersRef.current[liveIdxRef.current === 0 ? 1 : 0];
    if (!wantPlay || !idle.status.ready) return;
    // durante a mixagem a "próxima" JÁ está tocando na outra instância: engatilhar
    // a mesma uri aqui daria um segundo warm-play (mudo) e roubaria o slot do
    // overlay. Congela a pré-carga até o servidor avançar a fila.
    if (mixUri) return;
    if (nextIsYouTube && nextUri && nextUri !== npUri) {
      if (idle.status.currentUri !== nextUri) idle.cue(nextUri);
      return;
    }
    if (idle.status.currentUri) idle.standby();
  }, [
    wantPlay,
    mixUri,
    nextIsYouTube,
    nextUri,
    npUri,
    // `liveIdx` (state) entra só p/ re-rodar depois que o swap comita; a decisão
    // usa a ref. As duas instâncias entram nas deps porque qual delas é a ociosa
    // muda a cada faixa.
    liveIdx,
    p0.status.ready,
    p0.status.currentUri,
    p1.status.ready,
    p1.status.currentUri,
    p0.cue,
    p0.standby,
    p1.cue,
    p1.standby,
  ]);

  // 2) interpolação de posição: o player faz poll ~4x/s; entre leituras avançamos
  //    pelo relógio p/ o waveform não travar.
  const base = useRef({ pos: 0, at: 0, playing: false, uri: "" });
  useEffect(() => {
    base.current = {
      pos: status.position,
      at: Date.now(),
      playing: status.playing,
      uri: status.currentUri ?? "",
    };
  }, [status.position, status.playing, status.currentUri]);

  const [livePos, setLivePos] = useState(0);
  useEffect(() => {
    if (!npIsYouTube) {
      setLivePos(0);
      // sem `clearResume()` aqui: o registro é ÚNICO e compartilhado (0.2.26) —
      // limpar quando o now-playing não é YouTube apagaria a posição do motor que
      // ESTÁ no ar. Registro de outra faixa não faz mal: a uri não casa.
      return;
    }
    const tick = () => {
      const b = base.current;
      const p = b.playing ? b.pos + (Date.now() - b.at) : b.pos;
      const pos = status.duration > 0 ? Math.min(p, status.duration) : p;
      setLivePos(pos);
      if (npUri) saveResume(npUri, pos, b.playing);
    };
    tick();
    const id = setInterval(tick, 250);
    return () => clearInterval(id);
  }, [npIsYouTube, status.duration, npUri]);

  // 3) fim da faixa → avança a fila. Dirigido pelo EVENTO ENDED (endedNonce), não
  //    por posição (seek reseta a 0).
  const advancedFor = useRef<string>("");
  const lastEnded = useRef(0);
  useEffect(() => {
    if (!npIsYouTube || !wantPlay) return;
    if (status.endedNonce === lastEnded.current) return;
    lastEnded.current = status.endedNonce;
    if (
      status.endedUri &&
      status.endedUri === npUri &&
      advancedFor.current !== npUri
    ) {
      advancedFor.current = npUri;
      post("/api/v1/playback/next");
    }
  }, [npIsYouTube, wantPlay, status.endedNonce, status.endedUri, npUri, post]);

  // 3a) BLOQUEIO DE EMBED — o vídeo carregou metadados mas o dono desabilitou a
  //     incorporação (getVideoData().errorCode === "auth", família 150/101). Nunca
  //     produz som e o onError não dispara; sem isto o deck ficaria em silêncio a
  //     duração inteira até o fallback (3b). Detectado pelo player via prefixo
  //     `embed:` → pula NA HORA e avança a fila. O filtro de import (catalog.go)
  //     impede novos não-incorporáveis; isto cobre os já existentes na biblioteca.
  useEffect(() => {
    if (!npIsYouTube || !wantPlay) return;
    if (!status.error || !status.error.startsWith("embed:")) return;
    if (!npUri || advancedFor.current === npUri) return;
    // guarda-vazio: sem próxima na fila, NÃO pula — manter a faixa bloqueada
    // visível no deck em vez de sumir com tudo. Só avança quando há p/ onde ir.
    if (queueLen === 0) return;
    advancedFor.current = npUri;
    post("/api/v1/playback/next");
  }, [npIsYouTube, wantPlay, status.error, npUri, queueLen, post]);

  // 3b) fallback por DURAÇÃO — espelha o motor local/Spotify. O ENDED às vezes não
  //     chega (vídeo bloqueado, erro do IFrame, rede) → força o avanço passado
  //     durMs + folga se ninguém avançou por essa faixa. Recurso idêntico entre
  //     motores (hexagonal).
  useEffect(() => {
    if (!npIsYouTube || !wantPlay || userPaused || npDurMs <= 0) return;
    const id = setTimeout(() => {
      if (npUri && advancedFor.current !== npUri) {
        // guarda-vazio: se a faixa NUNCA tocou (bloqueio/erro) e a fila está
        // vazia, avançar esvaziaria o deck — segura. Se tocou (fim real que o
        // ENDED não reportou), pode avançar mesmo p/ fila vazia.
        if (queueLen === 0 && !playedThisUri.current) return;
        advancedFor.current = npUri;
        post("/api/v1/playback/next");
      }
    }, npDurMs + 1500);
    return () => clearTimeout(id);
  }, [npIsYouTube, wantPlay, userPaused, npUri, npDurMs, queueLen, post]);

  /* MIXAGEM CROSS-MOTOR — entrada: põe `uri` no ar AGORA, subindo de 0 até o
     ganho pleno no tempo do ⚙, POR CIMA da faixa que ainda toca no outro motor
     (Spotify). Aproveita a PRÉ-CARGA da v0.2.11: a instância engatilhada já tem o
     vídeo bufferizado e mudo, então `goLive()` sai som no mesmo instante — é o que
     torna a sobreposição possível sem buraco.

     Volume 0 é aplicado ANTES do goLive() (o cue estava mudo; unMute() devolveria
     o ganho anterior e daria um estouro de um frame). Só depois o ramp sobe.

     Caminho degradado (a próxima não estava engatilhada — fila mudou tarde):
     play(uri) na instância que já está no ar. Sem pré-carga vai haver buffering,
     mas é o mesmo custo do caminho ≤v0.2.10 e o ramp começa em 0 mesmo assim. */
  const startMix = useCallback(
    (uri: string): boolean => {
      if (!uri) return false;
      const idx = liveIdxRef.current;
      const cur = playersRef.current[idx];
      const other = playersRef.current[idx === 0 ? 1 : 0];
      const ms = fadeMs();
      if (other.status.ready && other.status.currentUri === uri) {
        cur.standby(); // instância ociosa (a faixa no ar é do outro motor)
        const nidx: 0 | 1 = idx === 0 ? 1 : 0;
        // ref ANTES do setState: o ramp abaixo já tem de mirar a nova instância.
        liveIdxRef.current = nidx;
        setLiveIdx(nidx);
        ytVol.current = 0;
        other.setVolume(0);
        other.goLive();
      } else {
        if (!cur.status.ready) return false;
        ytVol.current = 0;
        cur.setVolume(0);
        cur.play(uri);
      }
      armMix(uri);
      void rampVol(YT_TARGET, ms);
      return true;
    },
    [rampVol, armMix],
  );

  const toggle = useCallback(() => {
    if (!npIsYouTube) return;
    setUserPaused(status.playing);
    ytToggle();
  }, [npIsYouTube, status.playing, ytToggle]);

  const seek = useCallback(
    (frac: number) => {
      if (!npIsYouTube || status.duration <= 0) return;
      const ms = Math.max(0, Math.min(1, frac)) * status.duration;
      base.current = { pos: ms, at: Date.now(), playing: status.playing, uri: npUri };
      setLivePos(ms);
      if (ms < status.duration - 900) advancedFor.current = "";
      ytSeek(ms);
    },
    [npIsYouTube, status.duration, status.playing, npUri, ytSeek],
  );

  /* DETECTOR DE "NÃO SAIU SOM" por PROGRESSO (espelha o motor local).
     `status.mutedByPolicy` cobre só o caso em que o fallback mudo entrou; o IFrame
     também sabe ficar preso em UNSTARTED/CUED sem erro nenhum depois de um refresh.
     Sinal honesto: a posição não anda enquanto a estação diz que quer tocar. */
  const [stalled, setStalled] = useState(false);
  useEffect(() => {
    if (!npIsYouTube || !wantPlay || userPaused) {
      setStalled(false);
      return;
    }
    let last = -1;
    const id = setInterval(() => {
      const p = playersRef.current[liveIdxRef.current]?.status.position ?? 0;
      if (last < 0) {
        last = p;
        return; // 1ª amostra é referência
      }
      if (p !== last) {
        last = p;
        setStalled(false);
        return;
      }
      setStalled(true);
    }, 2500);
    return () => clearInterval(id);
  }, [npIsYouTube, wantPlay, userPaused, npUri]);

  const deck: YouTubeDeck = useMemo(
    () => ({
      isYouTube: npIsYouTube,
      live: npIsYouTube,
      playing: status.playing,
      posMs: livePos,
      durMs: status.duration,
      blocked: npIsYouTube && (status.mutedByPolicy || stalled),
    }),
    [npIsYouTube, status.playing, livePos, status.duration, status.mutedByPolicy, stalled],
  );

  // papéis por host: o que está no ar mira o slot do deck ao vivo; o engatilhado
  // mira o slot do deck ocioso (só quando de fato tem a próxima carregada —
  // senão fica parqueado). Overlay em page.tsx consome isso.
  const roles: [YouTubeHostRole, YouTubeHostRole] = useMemo(() => {
    const r: [YouTubeHostRole, YouTubeHostRole] = [null, null];
    if (npIsYouTube) r[liveIdx] = "live";
    else if (mixUri) {
      /* JANELA DA MIXAGEM: já estamos tocando `mixUri` mas o now-playing do
         servidor ainda é a faixa do OUTRO motor (só avança no fim natural dela).
         O host no ar segue no slot do deck OCIOSO — que é onde a próxima aparece
         pro operador. Sem isso os dois papéis dariam `null` e o iframe que está
         tocando seria parqueado (= deck preto, regressão da v0.2.9). Quando o
         servidor avançar, `npIsYouTube` vira true e o MESMO host troca p/ "live":
         muda só o retângulo do overlay (CSS), nunca o pai do iframe. */
      r[liveIdx] = "cue";
    }
    const ci: 0 | 1 = liveIdx === 0 ? 1 : 0;
    if (!mixUri && nextIsYouTube && cued.status.currentUri === nextUri)
      r[ci] = "cue";
    return r;
  }, [
    npIsYouTube,
    liveIdx,
    nextIsYouTube,
    nextUri,
    cued.status.currentUri,
    mixUri,
  ]);

  return { deck, toggle, seek, fadeOut, startMix, roles };
}
