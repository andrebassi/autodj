"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import {
  loadYouTubeIframe,
  youtubeVideoId,
  YT_ENDED,
  YT_PAUSED,
  YT_PLAYING,
  type YouTubePlayer,
} from "@/lib/youtubeIframe";
import { isBareIpOrigin, currentOrigin, localhostEquivalentURL } from "@/lib/ytOrigin";

export type YouTubePlayerStatus = {
  ready: boolean; // iframe player created + onReady fired
  playing: boolean;
  currentUri: string | null; // uri now loaded on the IFrame player
  position: number; // ms
  duration: number; // ms
  error: string | null;
  // fim natural da faixa: nonce++ a cada ENDED(0); endedUri = a que terminou. O
  // playout observa o nonce p/ avançar a fila (pos não serve: seek reseta a 0).
  endedNonce: number;
  endedUri: string | null;
  // autoplay gate: o iframe cross-origin do YouTube só toca COM SOM após ≥1 gesto
  // do usuário (política de autoplay). false até o 1º pointerdown/keydown do doc.
  // O playout só chama play() quando true — antes, o loadVideoById seria
  // bloqueado e o player travaria em -1 (unstarted). Ativação sticky vale o
  // documento inteiro, então um único gesto destrava todas as faixas seguintes.
  unlocked: boolean;
  /** está tocando MUDO só porque a política de autoplay barrou o som (fallback
   *  pós-refresh). Sai som no 1º gesto; a UI usa isto pra não anunciar "no ar". */
  mutedByPolicy: boolean;
};

const EMPTY: YouTubePlayerStatus = {
  ready: false,
  playing: false,
  currentUri: null,
  position: 0,
  duration: 0,
  error: null,
  endedNonce: 0,
  endedUri: null,
  unlocked: false,
  mutedByPolicy: false,
};

/* isolated YouTube IFrame Player controller (Story 6.7), mirror do
   useSpotifyPlayer. Toca um vídeo YouTube *dentro do browser* — embed/DRM, sem
   PCM, então nada aqui conecta ao deck/cartwall local ou ao bus de mix; as
   transições do auto-DJ usam só o volume próprio do IFrame (fade).

   DIVERGÊNCIA do Spotify (device remoto, sem elemento) e do local (<audio>
   destacado): o IFrame **exige um elemento DOM visível**. O host é um <div>
   persistente em page.tsx (RefObject), portalizado pro slot de capa do deck ao
   vivo (60×60). Criamos um filho não-React dentro do host — a IFrame API
   substitui esse filho pelo <iframe>, sem quebrar a reconciliação do React.

   NÃO há gate de premium: YouTube toca qualquer vídeo embeddable sem login. O
   gate de login (espelho fiel do Spotify) vive na aba/import, não no playout.

   PRÉ-CARGA (v0.2.11): o hook é instanciado DUAS vezes pelo playout — uma por
   host/deck. Só uma instância é a AO VIVO (`isLive`); a outra fica engatilhada
   (`cue`) com a próxima faixa já bufferizada e MUDA. Na virada o playout chama
   `standby()` na velha e `goLive()` na nova e troca os papéis — sem
   loadVideoById no instante da troca, então sem o buraco de 1-3s de buffering.
   `isLive` também trava o listener de gesto: sem isso, um clique qualquer faria
   o player engatilhado tocar ALTO por cima da faixa no ar. */
export function useYouTubePlayer(
  hostRef: RefObject<HTMLDivElement | null>,
  isLive: boolean = true,
): {
  status: YouTubePlayerStatus;
  /** carrega e toca `uri`. `startMs` > 1.5s carrega JÁ posicionado (retomada). */
  play: (uri: string, startMs?: number) => void;
  cue: (uri: string) => void;
  goLive: () => void;
  standby: () => void;
  toggle: () => void;
  stop: () => void;
  seek: (ms: number) => void;
  setVolume: (v: number) => void; // 0..1 (convertido p/ 0..100 do IFrame)
} {
  const [status, setStatus] = useState<YouTubePlayerStatus>(EMPTY);
  const playerRef = useRef<YouTubePlayer | null>(null);
  // uri carregada agora — a IFrame API não devolve a uri no evento ENDED, então
  // guardamos aqui p/ carimbar endedUri quando a faixa terminar.
  const currentUriRef = useRef<string>("");
  // a faixa carregada agora JÁ chegou a PLAYING alguma vez? Se sim, nunca é
  // bloqueio de embed (embed block nunca toca). Resetado a cada play().
  const playedRef = useRef(false);
  // timestamp em que a condição de bloqueio de embed começou a ser observada de
  // forma contínua. Só marcamos `embed:` após ~3s presos assim — evita o FALSO
  // POSITIVO do quirk da IFrame API: logo após loadVideoById o getVideoData()
  // devolve o errorCode STALE do vídeo ANTERIOR antes do novo carregar, o que
  // fazia a faixa "sumir" (pular na hora) mesmo sendo perfeitamente tocável.
  const embedBadSinceRef = useRef(0);
  // espelho de `isLive` p/ os listeners/callbacks (identidade estável, sem
  // re-registrar o listener de gesto a cada troca de papel).
  const liveRef = useRef(isLive);
  liveRef.current = isLive;
  // timers do aquecimento do cue (play mudo → pausa) e do self-heal do goLive.
  // Precisam ser cancelávies: se a faixa entrar no ar ANTES da pausa agendada, o
  // timer pausaria a música ao vivo.
  const warmRef = useRef<number[]>([]);
  const healRef = useRef<number | null>(null);
  // fallback de autoplay: timer que checa se destravou, e marca se estamos tocando
  // MUDO só por causa da política do browser (o gesto devolve o som).
  const mutedFallbackRef = useRef<number | null>(null);
  const mutedByPolicyRef = useRef(false);
  const clearWarm = useCallback((): void => {
    for (const t of warmRef.current) clearTimeout(t);
    warmRef.current = [];
    if (healRef.current !== null) {
      clearTimeout(healRef.current);
      healRef.current = null;
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    let poll: number | null = null;
    let waitTimer: number | null = null;
    let bootedHost: HTMLDivElement | null = null;
    let fitObserver: ResizeObserver | null = null;

    const boot = async (host: HTMLDivElement) => {
      bootedHost = host;
      try {
        const YT = await loadYouTubeIframe();
        if (cancelled) return;
        // PALCO FIXO 320×320 (safety de viewport, NÃO é a causa-raiz — essa era o
        // reparent do iframe, resolvido em v0.2.4 pelo overlay). O iframe renderiza
        // num palco 320×320 e é encolhido só VISUALMENTE via transform:scale pro
        // tamanho corrente do host (60×60 no slot). transform NÃO altera o box
        // interno do iframe → o player mede 320×320 (folga confortável), o visual
        // continua 60×60 no deck. O host recorta o excedente (overflow hidden).
        const STAGE = 320;
        host.style.position = "relative";
        host.style.overflow = "hidden";
        const stage = document.createElement("div");
        stage.style.position = "absolute";
        stage.style.top = "0";
        stage.style.left = "0";
        stage.style.width = STAGE + "px";
        stage.style.height = STAGE + "px";
        stage.style.transformOrigin = "top left";
        // filho não-React que a IFrame API substitui pelo <iframe> (o host
        // permanece sob controle do React). Fica DENTRO do palco 320×320.
        const inner = document.createElement("div");
        inner.style.width = "100%";
        inner.style.height = "100%";
        stage.appendChild(inner);
        host.appendChild(stage);

        // escala o palco pro tamanho corrente do host: 60px no slot ao vivo,
        // 0 parqueado (invisível, mas o iframe segue 320×320 internamente e o
        // áudio continua). Reescala sozinho quando o Deck move o host pro slot.
        const fit = () => {
          const w = host.clientWidth || 0;
          stage.style.transform = `scale(${w / STAGE})`;
        };
        fit();
        fitObserver = new ResizeObserver(fit);
        fitObserver.observe(host);

        const player = new YT.Player(inner, {
          width: "100%",
          height: "100%",
          // Fix do Erro 153 ("Video player configuration error"): desde dez/2025 o
          // YouTube exige que o embed se identifique via header HTTP Referer; embeds
          // sem referer (ou com Referrer-Policy restritivo) falham. O host
          // youtube-nocookie.com é o workaround oficial documentado, e o
          // referrerpolicy explícito no iframe (setado logo abaixo) garante o envio
          // do Referer mesmo se a página herdar uma política mais restrita.
          host: "https://www.youtube-nocookie.com",
          playerVars: {
            autoplay: 0,
            controls: 0,
            disablekb: 1,
            modestbranding: 1,
            rel: 0,
            playsinline: 1,
            fs: 0,
            iv_load_policy: 3,
            // origin + enablejsapi: recomendação oficial da IFrame Player API. Sem
            // `origin` o embed valida o postMessage contra "*", e desde 2025 os
            // players sem origin declarado passaram a falhar com "An error occurred.
            // Please try again later" (playback trava em UNSTARTED com metadata OK,
            // sem ser 153). Declarar a origem exata da página-pai faz o handshake do
            // player casar. enablejsapi=1 é setado pela API quando há eventos, mas o
            // deixamos explícito p/ garantir o canal de controle JS.
            origin:
              typeof window !== "undefined" ? window.location.origin : "",
            enablejsapi: 1,
          },
          events: {
            onReady: () => {
              if (!cancelled) setStatus((s) => ({ ...s, ready: true, error: null }));
            },
            onStateChange: (e) => {
              const st = e.data;
              if (st === YT_PLAYING) {
                // chegou a tocar → esta faixa NÃO é bloqueio de embed. Trava o
                // detector p/ o resto da faixa e limpa qualquer strike em curso.
                playedRef.current = true;
                embedBadSinceRef.current = 0;
                setStatus((s) => ({ ...s, playing: true }));
              } else if (st === YT_PAUSED) {
                setStatus((s) => ({ ...s, playing: false }));
              } else if (st === YT_ENDED) {
                setStatus((s) => ({
                  ...s,
                  playing: false,
                  endedNonce: s.endedNonce + 1,
                  endedUri: currentUriRef.current || s.currentUri,
                }));
              }
            },
            onError: (e) => {
              // Só marcamos `embed:` (→ playout PULA na hora, effect 3a) p/ códigos
              // PERMANENTEMENTE mortos: 100 (removido/privado), 101 & 150 (incorporação
              // desabilitada pelo dono). Esses NUNCA tocam nem YouTube-first, então o
              // pulo é sempre correto e nunca é falso-positivo na troca Spotify→YouTube.
              // Códigos 2/5 (parâmetro inválido / erro HTML5) são AMBÍGUOS/transitórios
              // — especialmente na troca de motor (sem gesto no instante do load) o
              // player pode emitir e ainda assim tocar. NÃO marcamos `embed:` neles
              // (senão o deck pretava e a faixa boa pulava): viram erro cosmético `yt:`,
              // sem esconder o overlay nem avançar a fila; o fallback por duração (3b)
              // cobre um 2/5 que seja de fato terminal. `play()` zera o error por faixa.
              const dead = e.data === 100 || e.data === 101 || e.data === 150;
              // Origem IP (dev) transforma TODO clipe monetizado em 150/"auth" —
              // ver lib/ytOrigin.ts. Sem esse aviso o sintoma é só "a faixa some".
              if (dead && isBareIpOrigin()) {
                console.warn(
                  `[autodj] YouTube erro ${e.data} em origem IP (${currentOrigin()}): ` +
                    `clipes musicais só incorporam a partir de um domínio. Abra em ${localhostEquivalentURL()}`,
                );
              }
              setStatus((s) => ({ ...s, error: dead ? `embed:${e.data}` : `yt:${e.data}` }));
            },
          },
        });
        playerRef.current = player;

        // Erro 153 hardening: a IFrame API substitui `inner` por um <iframe>. Sem
        // o atributo referrerpolicy explícito, o iframe herda a política do
        // documento — se algum dia a página passar a mandar Referrer-Policy
        // restrita (no-referrer/same-origin), o embed do YouTube quebra com 153.
        // Fixamos strict-origin-when-cross-origin (recomendação do YouTube) assim
        // que o iframe aparece; tentamos no tick atual e no próximo pois a criação
        // pode ser assíncrona conforme a versão da API.
        const pinReferrer = () => {
          const ifr = host.querySelector("iframe");
          if (ifr) ifr.setAttribute("referrerpolicy", "strict-origin-when-cross-origin");
        };
        pinReferrer();
        window.setTimeout(pinReferrer, 0);

        // a IFrame API não emite position; poll leve p/ o waveform/timers do deck
        // seguirem a música (getCurrentTime/getDuration em segundos → ms).
        // Também detecta o bloqueio de embed que o onError NÃO dispara: vídeos
        // "- Topic"/restritos carregam metadados mas ficam presos em UNSTARTED(-1)
        // com getVideoData().errorCode === "auth" (família 150/101), sem som e sem
        // erro. Sinalizamos com o prefixo `embed:` p/ o playout pular na hora em
        // vez de esperar a duração inteira em silêncio.
        poll = window.setInterval(() => {
          const p = playerRef.current;
          if (!p || typeof p.getCurrentTime !== "function") return;
          try {
            const pos = Math.round(p.getCurrentTime() * 1000);
            const dur = Math.round(p.getDuration() * 1000);
            let embedErr: string | null = null;
            // detector de bloqueio de embed COM janela de graça + guarda de
            // staleness. Só marca `embed:` se: (a) a faixa NUNCA tocou; (b) está
            // presa em estado parado (-1/3/5); (c) o errorCode é de bloqueio; (d) o
            // video_id do getVideoData casa com o carregado (não é dado STALE do
            // vídeo anterior, quirk que fazia a faixa boa "sumir" na hora); (e) a
            // condição persiste por ≥3s. Vídeo tocável chega a PLAYING em 1-2s e
            // nunca marca; só o realmente bloqueado fica travado 3s → pula.
            if (
              !playedRef.current &&
              typeof p.getVideoData === "function" &&
              typeof p.getPlayerState === "function"
            ) {
              const st = p.getPlayerState();
              if (st === -1 || st === 3 || st === 5) {
                const vd = p.getVideoData();
                const code = vd?.errorCode;
                const loadedId = youtubeVideoId(currentUriRef.current);
                // fresh = metadados já são do vídeo carregado (ou ainda sem id).
                const fresh = !loadedId || !vd?.video_id || vd.video_id === loadedId;
                const blocked =
                  code === "auth" || code === "150" || code === "101" || code === "100";
                if (fresh && blocked) {
                  if (embedBadSinceRef.current === 0) embedBadSinceRef.current = Date.now();
                  if (Date.now() - embedBadSinceRef.current >= 3000) embedErr = `embed:${code}`;
                } else {
                  embedBadSinceRef.current = 0;
                }
              } else {
                embedBadSinceRef.current = 0;
              }
            }
            setStatus((s) => ({
              ...s,
              position: pos,
              duration: dur > 0 ? dur : s.duration,
              error: embedErr ?? (s.error && s.error.startsWith("embed:") ? null : s.error),
            }));
          } catch {
            /* player ainda inicializando — ignora */
          }
        }, 250);
      } catch (e) {
        if (!cancelled) {
          setStatus((s) => ({
            ...s,
            error: e instanceof Error ? e.message : "erro ao carregar IFrame API",
          }));
        }
      }
    };

    // o host só é renderizado DEPOIS do gate de login (page.tsx), então no mount
    // hostRef.current costuma ser null. Um `if (!host) return` mataria o boot pra
    // sempre (deps [hostRef] são estáveis → o effect não re-dispara quando o ref
    // preenche). Esperamos o host aparecer e então bootamos uma vez.
    const waitForHost = () => {
      if (cancelled) return;
      const host = hostRef.current;
      if (host) {
        void boot(host);
        return;
      }
      waitTimer = window.setTimeout(waitForHost, 100);
    };
    waitForHost();

    return () => {
      cancelled = true;
      if (poll !== null) clearInterval(poll);
      if (waitTimer !== null) clearTimeout(waitTimer);
      clearWarm();
      fitObserver?.disconnect();
      try {
        playerRef.current?.destroy();
      } catch {
        /* ignora */
      }
      playerRef.current = null;
      currentUriRef.current = "";
      if (bootedHost) bootedHost.innerHTML = "";
    };
  }, [hostRef, clearWarm]);

  // Autoplay policy: o iframe cross-origin do YouTube não inicia COM SOM sem um
  // gesto do usuário. O playout cuea o vídeo cedo (loadVideoById); quando o
  // autoplay é bloqueado ele fica em UNSTARTED(-1)/CUED(5)/BUFFERING(3). Este
  // listener PERSISTENTE captura QUALQUER gesto (pointerdown/keydown) e, se houver
  // um vídeo cued/travado, chama playVideo() SÍNCRONO na janela de ativação →
  // toca com som. NÃO mexe em PAUSED(2) (pausa intencional via toggle) nem em
  // PLAYING(1) (no-op). Um único gesto já basta pra 1ª faixa; as seguintes tocam
  // sozinhas (o frame ganha engagement após o 1º play com gesto). capture:true
  // pega o gesto antes de stopPropagation da UI.
  useEffect(() => {
    const onGesture = () => {
      setStatus((s) => (s.unlocked ? s : { ...s, unlocked: true }));
      // SÓ a instância ao vivo destrava por gesto. Na engatilhada, playVideo()
      // aqui tocaria a PRÓXIMA faixa por cima da atual (o cue fica pausado de
      // propósito em CUED/PAUSED) — o unlock sticky do documento vale pras duas.
      if (!liveRef.current) return;
      const p = playerRef.current;
      if (!p || typeof p.getPlayerState !== "function") return;
      try {
        // pode estar TOCANDO MUDO (fallback de autoplay, abaixo): aí o gesto só
        // precisa devolver o som — pausar/re-tocar cortaria o áudio à toa.
        if (mutedByPolicyRef.current) {
          mutedByPolicyRef.current = false;
          p.unMute();
          setStatus((s) => ({ ...s, mutedByPolicy: false }));
        }
        const st = p.getPlayerState();
        if (st === -1 || st === 3 || st === 5) p.playVideo();
      } catch {
        /* player ainda inicializando — ignora */
      }
    };
    const opts: AddEventListenerOptions = { capture: true };
    window.addEventListener("pointerdown", onGesture, opts);
    window.addEventListener("keydown", onGesture, opts);
    return () => {
      window.removeEventListener("pointerdown", onGesture, opts);
      window.removeEventListener("keydown", onGesture, opts);
    };
  }, []);

  /* `startMs` existe pra RETOMADA pós-refresh (0.2.26): o IFrame é recriado no
     reload e `loadVideoById(id)` sempre recomeça do 00:00. A API aceita a posição
     na forma de objeto (`startSeconds`), que é a única maneira de já carregar
     posicionado — dar `seekTo` depois faria o vídeo tocar do início por um instante
     antes de pular. */
  const play = useCallback(
    (uri: string, startMs = 0): void => {
      const p = playerRef.current;
      const id = youtubeVideoId(uri);
      if (!p || !id) {
        setStatus((s) => ({ ...s, error: "player YouTube não pronto" }));
        return;
      }
      clearWarm();
      currentUriRef.current = uri;
      // nova faixa → zera detector de bloqueio (senão herdaria strike/played da
      // faixa anterior).
      playedRef.current = false;
      embedBadSinceRef.current = 0;
      try {
        // pode vir de um ciclo de cue (que muta p/ bufferizar) — desmuta sempre.
        p.unMute();
      } catch {
        /* API antiga sem unMute — ignora */
      }
      if (startMs > 1500) p.loadVideoById({ videoId: id, startSeconds: startMs / 1000 });
      else p.loadVideoById(id); // autoplays
      setStatus((s) => ({ ...s, currentUri: uri, error: null }));

      /* FALLBACK MUDO (0.2.26): se em 1,2s o vídeo não estiver tocando, quem
         barrou foi a política de autoplay — carregar COM som exige gesto na página
         nova, e um refresh não é gesto. Tocar MUDO é sempre permitido, então a
         faixa segue rodando na posição certa e o `onGesture` acima devolve o som
         no primeiro clique. Sem isso, um Cmd+R deixava a estação parada.
         BUFFERING(3) conta como "andando" — é carregamento, não bloqueio. */
      if (mutedFallbackRef.current) window.clearTimeout(mutedFallbackRef.current);
      mutedFallbackRef.current = window.setTimeout(() => {
        mutedFallbackRef.current = null;
        const pp = playerRef.current;
        if (!pp || typeof pp.getPlayerState !== "function") return;
        try {
          const st = pp.getPlayerState();
          if (st === YT_PLAYING || st === 3) return;
          pp.mute();
          mutedByPolicyRef.current = true;
          setStatus((s) => ({ ...s, mutedByPolicy: true }));
          pp.playVideo();
        } catch {
          /* ignora */
        }
      }, 1200);
    },
    [clearWarm],
  );

  /* PRÉ-CARGA: engatilha `uri` neste player (que NÃO está no ar) e bufferiza.
     `cueVideoById` sozinho não basta — a doc oficial é explícita: "The player
     does not request the FLV until playVideo() or seekTo() is called", ou seja,
     ele só baixa thumbnail + metadados. Então damos um play MUDO curto e
     pausamos de volta no 0: autoplay MUDO é sempre permitido pela política do
     browser, e o segmento de mídia fica em buffer. Na virada, goLive() só
     desmuta e dá play — sem request novo, sem buraco de silêncio. */
  const cue = useCallback(
    (uri: string): void => {
      const p = playerRef.current;
      const id = youtubeVideoId(uri);
      if (!p || !id || typeof p.cueVideoById !== "function") return;
      if (currentUriRef.current === uri) return; // já engatilhado
      clearWarm();
      currentUriRef.current = uri;
      playedRef.current = false;
      embedBadSinceRef.current = 0;
      try {
        p.mute();
        p.cueVideoById(id);
      } catch {
        return;
      }
      warmRef.current.push(
        window.setTimeout(() => {
          try {
            playerRef.current?.mute();
            playerRef.current?.playVideo();
          } catch {
            /* ignora */
          }
        }, 300),
        window.setTimeout(() => {
          try {
            playerRef.current?.pauseVideo();
            playerRef.current?.seekTo(0, true);
          } catch {
            /* ignora */
          }
        }, 2600),
      );
      setStatus((s) => ({ ...s, currentUri: uri, error: null }));
    },
    [clearWarm],
  );

  /* vira o player engatilhado em AO VIVO: volta ao 0, desmuta e toca. Self-heal
     em 1.8s — se por qualquer motivo não estiver tocando (buffer descartado,
     gesto ainda não dado, quirk da API), recarrega via loadVideoById, que é
     exatamente o caminho ≤v0.2.10. A pré-carga nunca pode regredir o playout. */
  const goLive = useCallback((): void => {
    const p = playerRef.current;
    if (!p) return;
    clearWarm();
    try {
      p.seekTo(0, true);
      p.unMute();
      p.playVideo();
    } catch {
      /* ignora */
    }
    setStatus((s) => ({ ...s, error: null }));
    healRef.current = window.setTimeout(() => {
      healRef.current = null;
      const pp = playerRef.current;
      if (!pp || typeof pp.getPlayerState !== "function") return;
      try {
        if (pp.getPlayerState() === YT_PLAYING) return;
        const id = youtubeVideoId(currentUriRef.current);
        if (!id) return;
        pp.unMute();
        pp.loadVideoById(id);
      } catch {
        /* ignora */
      }
    }, 1800);
  }, [clearWarm]);

  /* tira este player do ar (a faixa dele acabou/foi trocada): pausa, muta e
     esquece a uri, liberando a instância p/ engatilhar a próxima. Não destrói
     nem reparenta nada — o iframe segue vivo no mesmo host (invariante desde
     v0.2.4). */
  const standby = useCallback((): void => {
    clearWarm();
    const p = playerRef.current;
    try {
      p?.pauseVideo();
      p?.mute();
    } catch {
      /* ignora */
    }
    currentUriRef.current = "";
    playedRef.current = false;
    embedBadSinceRef.current = 0;
    setStatus((s) => ({ ...s, playing: false, currentUri: null, error: null }));
  }, [clearWarm]);

  const toggle = useCallback((): void => {
    const p = playerRef.current;
    if (!p) return;
    const st = p.getPlayerState();
    if (st === YT_PLAYING) p.pauseVideo();
    else p.playVideo();
  }, []);

  const stop = useCallback((): void => {
    playerRef.current?.pauseVideo();
    setStatus((s) => ({ ...s, playing: false }));
  }, []);

  // seek absoluto (ms) no IFrame — usado pelo deck p/ retomar posição no refresh
  // e p/ o seek do waveform. Atualiza a posição local otimista.
  const seek = useCallback((ms: number): void => {
    const p = playerRef.current;
    if (!p) return;
    const pos = Math.max(0, Math.round(ms));
    p.seekTo(pos / 1000, true);
    setStatus((s) => ({ ...s, position: pos }));
  }, []);

  // volume 0..1 (base do fade-in/out do playout) → 0..100 do IFrame. Ganho local
  // do player, sem rede.
  const setVolume = useCallback((v: number): void => {
    playerRef.current?.setVolume(Math.max(0, Math.min(1, v)) * 100);
  }, []);

  return { status, play, cue, goLive, standby, toggle, stop, seek, setVolume };
}
