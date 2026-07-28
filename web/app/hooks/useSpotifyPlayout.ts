"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { NowPlaying } from "@/lib/types";
import { fetchSpotifyStatus, SpotifyNotConfigured } from "@/lib/api";
import { fadeEnabled, fadeMs } from "@/lib/fade";
import { resumeFor, saveResume } from "@/lib/resume";
import { useSpotifyPlayer } from "@/hooks/useSpotifyPlayer";

/* posição do playout persistida no browser p/ RETOMAR após refresh.
   Spotify toca via device remoto do SDK; ao dar refresh, um device novo é criado
   e `play(uri)` recomeçaria do zero. Guardamos {uri,pos,at,playing} e, na 1ª
   carga, se a faixa no ar for a mesma, posicionamos onde estava (pos + tempo
   decorrido) — a música continua em vez de reiniciar. */
/* As funções de resume moravam AQUI e valiam só pro Spotify. Foram pra
   `lib/resume.ts` e agora servem aos três motores — local e YouTube não retomavam
   nada (F5 reiniciava a música do 00:00). */

// fade de transição (PRÓXIMA/RETROCEDER): faixa nova entra subindo e a que sai
// desce, em vez do corte seco. A DURAÇÃO vem do ⚙ Modo do Player (`fadeMs()`:
// 0/3/5/7s) e é lida NO MOMENTO do fade — antes era uma constante 3000 aqui, que
// ignorava a escolha do operador. SPOT_TARGET = ganho pleno do device do SDK
// (espelha o volume de criação do player, p/ não mudar a sonoridade).
const SPOT_TARGET = 0.8;

export type PlayoutDeck = {
  /** now-playing é Spotify e está sendo dirigido pelo SDK do browser */
  isSpotify: boolean;
  /** há feed REAL dirigindo a posição do deck (Spotify via SDK OU local via
   *  <audio>) — generaliza `isSpotify` p/ os dois motores. */
  live: boolean;
  /** SDK realmente tocando (não pausado) */
  playing: boolean;
  /** posição interpolada ao vivo (ms) — dirige o waveform do Deck A */
  posMs: number;
  /** duração real reportada pelo SDK (ms) */
  durMs: number;
  /** o BROWSER recusou tocar (autoplay bloqueado depois de um refresh). A estação
   *  quer tocar (`wantPlay`) mas não sai som até um gesto. A UI usa isto pra virar
   *  o botão do deck em ▶ — deixá-lo em ⏹ mente ("parece no ar") e, pior, o clique
   *  natural de quem quer som PARARIA a estação. */
  blocked?: boolean;
  /** FIM EFETIVO da faixa (ms) — normalmente igual a `durMs`, mas MENOR quando o
   *  trim corta o vazio do fim. Quem coordena a mixagem tem de usar este número:
   *  com `durMs` cru o crossfade começaria DEPOIS do fim já cortado, e a virada
   *  viraria corte seco justamente nas faixas com silêncio no fim. Ausente ⇒ sem
   *  trim (streaming), aí `durMs` é o fim. `durMs` continua sendo a duração REAL,
   *  porque é ela que a tela mostra. */
  endMs?: number;
  /** entrada do vocal (ms) da faixa no ar; 0 = não marcado. Alimenta o LOOP e a
   *  marca 🎤 do deck. Só faixa local. */
  vocalMs?: number;
  /** o INSTRUMENTAL separado por IA já está pronto para a faixa no ar? É o que
   *  decide se o botão 🚫🎤 VOZ pode ligar — ele troca a fonte pelo arquivo sem
   *  vocal, então sem preparo não há o que tocar. Só faixa local; lido AO VIVO do
   *  endpoint, nunca do `Playable` (que é um retrato do enfileiramento). */
  instrumentalReady?: boolean;
};

/* Driver de playout do Spotify no nível da página (Story 6.3 → playout real).

   A fila é a agenda; quando o now-playing é uma faixa Spotify, ela toca
   *out-of-band* pelo Web Playback SDK (DRM, sem PCM — nunca no bus Liquidsoap).
   Este hook faz a ponte entre o now-playing do servidor (WebSocket) e o device
   Spotify do browser:

   1. quando `snap.now_playing.source === "spotify"` e a URI muda → `play(uri)`;
   2. interpola a posição entre os eventos `player_state_changed` (que só chegam
      em play/pause/seek) p/ o waveform seguir a música de verdade;
   3. detecta o fim da faixa e chama `POST /playback/next` — a fila avança
      sozinha, faixa após faixa (hard_cut), como combinado.

   Gated por Premium: sem Premium o SDK recusa e o hook fica inerte (isSpotify=false),
   caindo no placeholder visual do deck. Usa um device separado ("AutoDJ Playout")
   do preview do painel p/ não brigarem pelo mesmo id. */
export function useSpotifyPlayout(
  snap: NowPlaying | null,
  post: (path: string) => void,
  wantPlay = true,
  // chamado quando o Spotify passa a tocar em OUTRO device da conta (handoff) —
  // a página desliga o PLAY mestre p/ não competir com o outro aparelho.
  onExternalTakeover?: () => void,
  // chamado quando a faixa entra no deck e o Spotify NUNCA sai do zero — device
  // conectado, comandos aceitos, áudio nenhum. Ver STALL_MS abaixo.
  onStalled?: () => void,
): {
  deck: PlayoutDeck;
  toggle: () => void;
  seek: (frac: number) => void;
  fadeOut: () => Promise<void>;
  startMix: (uri: string) => boolean;
} {
  const [premium, setPremium] = useState(false);
  const player = useSpotifyPlayer(premium, "AutoDJ Playout");
  const {
    status,
    play,
    toggle: sdkToggle,
    stop: sdkStop,
    seek: sdkSeek,
    setVolume,
  } = player;

  // ramp de volume do device SDK (fade). Cancela ramp em voo. spotVol espelha o
  // último ganho aplicado (base do próximo ramp). Passos de 100ms (sem rede).
  const spotVol = useRef(SPOT_TARGET);
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
        const from = spotVol.current;
        if (ms <= 0 || Math.abs(to - from) < 0.01) {
          spotVol.current = to;
          void setVolume(to);
          resolve();
          return;
        }
        const steps = Math.max(1, Math.round(ms / 100));
        let i = 0;
        fadeTimer.current = window.setInterval(() => {
          i++;
          const v = Math.max(0, Math.min(1, from + (to - from) * (i / steps)));
          spotVol.current = v;
          void setVolume(v);
          if (i >= steps) {
            clearFade();
            spotVol.current = to;
            void setVolume(to);
            resolve();
          }
        }, 100);
      }),
    [clearFade, setVolume],
  );
  // fade-in: faixa nova arranca em 0 e sobe até o ganho pleno no tempo do ⚙. Fade
  // desligado (⚙ Configurações) → entra a ganho pleno direto (corte seco).
  const fadeIn = useCallback(async () => {
    if (!fadeEnabled()) {
      spotVol.current = SPOT_TARGET;
      await setVolume(SPOT_TARGET);
      return;
    }
    spotVol.current = 0;
    await setVolume(0);
    await rampVol(SPOT_TARGET, fadeMs());
  }, [setVolume, rampVol]);
  // fade-out: desce o ganho até 0 no tempo do ⚙ (chamado antes de trocar a faixa
  // no ar). Fade desligado → no-op (troca instantânea, como o STOP).
  const fadeOut = useCallback(async () => {
    if (!status.playing || !fadeEnabled()) return;
    await rampVol(0, fadeMs());
  }, [status.playing, rampVol]);
  useEffect(() => () => clearFade(), [clearFade]);

  // premium gate — poll leve (status do backend). Sem Premium/desconectado o
  // SDK nem instancia e o deck cai no placeholder.
  useEffect(() => {
    let stop = false;
    const load = async () => {
      try {
        const s = await fetchSpotifyStatus();
        if (stop) return;
        setPremium(s !== SpotifyNotConfigured && s.connected && s.premium);
      } catch {
        /* backend offline → mantém último estado */
      }
    };
    load();
    const id = setInterval(load, 8000);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, []);

  const np = snap?.now_playing;
  const npIsSpotify = !!np && np.source === "spotify" && !!np.uri && !!np.track_id;
  const npUri = npIsSpotify ? np!.uri : "";
  const npDurMs = np?.duration_ms ?? 0;

  // PAUSE do deck = intent de segurar a faixa (distinto do wantPlay mestre).
  // Pausar DEVE congelar o auto-avanço: sem isso o fallback por duração (só
  // olhava wantPlay) disparava /playback/next mesmo pausado → avançava e a
  // próxima re-tocava, "tirando" o pause. Reseta a cada troca de faixa.
  const [userPaused, setUserPaused] = useState(false);
  useEffect(() => {
    setUserPaused(false);
  }, [npUri]);

  // resume: registro único compartilhado (`lib/resume.ts`), consumido na 1ª faixa
  // carregada depois do boot que casar com a uri. Transição normal de faixa começa
  // do zero porque `resumeParaFaixa` só devolve valor uma vez.

  // MIXAGEM CROSS-MOTOR: faixa que já foi iniciada AQUI *antes* de virar o
  // now-playing (começou por cima do fim da faixa do outro motor). Quando o
  // servidor finalmente avança a fila e ela vira o now-playing, o effect 1 tem de
  // PULAR o play() — senão a música reiniciaria do zero no meio da mixagem.
  const preStarted = useRef<string>("");

  // 1) now-playing Spotify mudou → carrega no device. lastPlayed evita re-disparo
  //    a cada snapshot do WS (que reemite o mesmo now-playing).
  const lastPlayed = useRef<string>("");
  useEffect(() => {
    // Só carrega/toca com o PLAY mestre ligado (wantPlay). Parado/cued (nada no
    // ar, ou AUTO OFF esperando o operador) NÃO arranca sozinho — a faixa fica
    // engatilhada no deck a 00:00 até darem PLAY.
    if (!npIsSpotify || !status.ready || !wantPlay) return;
    if (npUri === lastPlayed.current) return;
    // esta faixa JÁ está tocando aqui (pré-início da mixagem cross-motor) — o
    // servidor só agora avançou a fila. Adota o trilho sem re-tocar.
    if (npUri && npUri === preStarted.current) {
      preStarted.current = "";
      lastPlayed.current = npUri;
      return;
    }
    lastPlayed.current = npUri;
    // retoma na posição salva se for a MESMA faixa que tocava antes do refresh.
    const resumeMs = resumeFor(npUri, npDurMs);
    void (async () => {
      await play(npUri);
      if (resumeMs > 1500) {
        // retomada pós-refresh: não é troca de faixa — vai a volume pleno direto
        // (sem fade) e reposiciona onde estava.
        spotVol.current = SPOT_TARGET;
        void setVolume(SPOT_TARGET);
        // pequena espera p/ o device novo assumir o playback, então posiciona.
        await new Promise((r) => setTimeout(r, 700));
        base.current = { pos: resumeMs, at: Date.now(), playing: true, uri: npUri };
        setLivePos(resumeMs);
        void sdkSeek(resumeMs);
      } else {
        // faixa nova entrando no ar → fade-in de 3s (sem corte seco).
        void fadeIn();
      }
    })();
  }, [npIsSpotify, npUri, npDurMs, status.ready, wantPlay, play, sdkSeek, setVolume, fadeIn]);

  // PLAY mestre desligado (nada no ar / cued) → não deixa o SDK tocando e re-arma
  // o trilho, p/ que dar PLAY de novo recarregue a faixa da cabeça. sdkStop
  // incondicional (é idempotente): se status.playing estiver stale — playback
  // migrou pra outro device e este SDK "acha" que não toca — ainda assim garante
  // o pause do device do SDK. O device REMOTO é calado pelo Connect pause no STOP.
  useEffect(() => {
    if (wantPlay) return;
    void sdkStop();
    lastPlayed.current = "";
  }, [wantPlay, sdkStop]);

  // quando o now-playing deixa de ser Spotify (ou fila esvazia), solta o trilho
  // p/ que a mesma faixa possa ser recarregada se voltar.
  useEffect(() => {
    if (!npIsSpotify) lastPlayed.current = "";
  }, [npIsSpotify]);

  // ABORTO DA MIXAGEM: a faixa pré-iniciada TEM de virar o now-playing logo em
  // seguida. Se entrou OUTRA no ar (operador mexeu na fila / tocar-agora no meio
  // da mixagem), o pré-início vira música fantasma tocando por cima — cala e
  // solta o trilho. Se a que entrou for Spotify, o effect 1 já a carrega por cima
  // (não precisa parar).
  const npAnyUri = np?.uri ?? "";
  useEffect(() => {
    const pre = preStarted.current;
    if (!pre || !npAnyUri || npAnyUri === pre) return;
    preStarted.current = "";
    if (!npIsSpotify) void sdkStop();
  }, [npAnyUri, npIsSpotify, sdkStop]);

  // 2) interpolação de posição: o SDK só emite position nos eventos de estado.
  //    Entre eles, avançamos pelo relógio p/ o waveform não travar.
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
    if (!npIsSpotify) {
      setLivePos(0);
      // NÃO apagar o registro aqui. Ele é ÚNICO e compartilhado com os outros
      // motores (0.2.26): este `clearResume()` incondicional rodava toda vez que o
      // now-playing NÃO era Spotify e destruía a posição que o motor local/YouTube
      // tinha acabado de salvar — resume dos outros nunca funcionaria. Registro
      // velho é inofensivo: só se aplica com a uri igual e dentro do frescor.
      return;
    }
    const tick = () => {
      const b = base.current;
      const p = b.playing ? b.pos + (Date.now() - b.at) : b.pos;
      const pos = status.duration > 0 ? Math.min(p, status.duration) : p;
      setLivePos(pos);
      // persiste ~1x/s (o throttle mora no módulo) p/ o refresh retomar perto.
      if (npUri) saveResume(npUri, pos, b.playing);
    };
    tick();
    const id = setInterval(tick, 250);
    return () => clearInterval(id);
  }, [npIsSpotify, status.duration, npUri]);

  // 3) fim da faixa → avança a fila. Dirigido pelo EVENTO de fim do SDK
  //    (`endedNonce`), não por posição: ao terminar, o Spotify reseta
  //    position→0 e paused=true, então "pos ≥ duração" nunca dispararia (era o
  //    bug: faixa acabava e não avançava). O player emite endedNonce++ com a
  //    endedUri da faixa terminada; avançamos uma vez, só se casar com o
  //    now-playing e ainda não tivermos avançado por ele.
  const advancedFor = useRef<string>("");
  const lastEnded = useRef(0);

  /* ── Detector de motor TRAVADO (v0.2.24) ────────────────────────────────────
     Sintoma real, medido em produção: o device do Spotify conecta, a faixa é
     carregada nele (`getCurrentState()` devolve o track certo) e fica
     `paused:true, pos:0` para sempre. `togglePlay()`, `activateElement()` e
     `resume()` todos RESOLVEM SEM ERRO — o SDK aceita tudo e não toca. A causa
     estava fora do app (o Spotify passou a responder 403 no `/v1/me`), mas o
     estrago foi nosso: sem áudio e sem aviso, e o fallback por duração (3b) ia
     pulando faixa por faixa até esvaziar a fila. O operador via "mix mudo e
     depois some tudo".

     `progressed` marca que ESTA faixa realmente andou (posição passou de
     PROGRESS_MIN). Enquanto não andar:
       - 3b NÃO avança (nunca começou; "acabar" não faz sentido);
       - passado STALL_MS, avisamos uma única vez por URI via onStalled. */
  const PROGRESS_MIN = 1500;
  const STALL_MS = 9000;
  const progressed = useRef(false);
  const stalledFor = useRef<string>("");

  // zera o rastreio a cada faixa nova
  useEffect(() => {
    progressed.current = false;
  }, [npUri]);

  // marca progresso real assim que a posição sai do zero
  useEffect(() => {
    if (npIsSpotify && status.position > PROGRESS_MIN) progressed.current = true;
  }, [npIsSpotify, status.position]);

  // avisa (uma vez por faixa) quando o motor não sai do lugar
  useEffect(() => {
    if (!npIsSpotify || !wantPlay || userPaused || !npUri) return;
    if (stalledFor.current === npUri) return;
    const id = setTimeout(() => {
      if (progressed.current || stalledFor.current === npUri) return;
      stalledFor.current = npUri;
      onStalled?.();
    }, STALL_MS);
    return () => clearTimeout(id);
  }, [npIsSpotify, wantPlay, userPaused, npUri, onStalled]);
  useEffect(() => {
    // só avança com o PLAY mestre ligado — estação parada/cued NÃO deve escoar a
    // fila sozinha ao "terminar" faixa.
    if (!npIsSpotify || !wantPlay) return;
    if (status.endedNonce === lastEnded.current) return;
    lastEnded.current = status.endedNonce;
    if (
      status.endedUri &&
      status.endedUri === npUri &&
      advancedFor.current !== npUri
    ) {
      // "Acabou" sem NUNCA ter tocado não é fim de faixa — é motor travado. O SDK
      // emite esse evento quando a reprodução não vinga (device recusado, conta sem
      // Premium, faixa indisponível no mercado), e avançar aqui esvazia a fila
      // faixa a faixa em silêncio: foi assim que a programação sumia. Este é o
      // sinal MAIS RÁPIDO de travamento — mais que o timeout de 9s —, então serve
      // de gatilho direto do aviso.
      if (!progressed.current) {
        if (stalledFor.current !== npUri) {
          stalledFor.current = npUri;
          onStalled?.();
        }
        return;
      }
      advancedFor.current = npUri;
      post("/api/v1/playback/next");
    }
  }, [npIsSpotify, wantPlay, status.endedNonce, status.endedUri, npUri, post, onStalled]);

  // 3b) fallback por DURAÇÃO — espelha o motor local (useLocalPlayout). O evento
  //     `endedNonce` do SDK às vezes não chega (device desconectado após restart
  //     do server, sem Premium ativo, rede) → a faixa "acabava" e ficava presa no
  //     deck, sem avançar (bug #63). Aqui, passado `durMs + folga`, forçamos o
  //     avanço se ninguém avançou por essa faixa ainda. Mesmo comportamento p/
  //     qualquer provider (hexagonal: recurso idêntico Spotify/YouTube/local).
  useEffect(() => {
    if (!npIsSpotify || !wantPlay || userPaused || npDurMs <= 0) return;
    const id = setTimeout(() => {
      // Só avança se a faixa REALMENTE tocou. Sem isso, motor travado (posição
      // presa em 0) fazia este timer pular faixa por faixa até esvaziar a fila —
      // "acabou" e "nunca começou" viravam a mesma coisa. Ver o detector acima.
      if (!progressed.current) return;
      if (npUri && advancedFor.current !== npUri) {
        advancedFor.current = npUri;
        post("/api/v1/playback/next");
      }
    }, npDurMs + 1500);
    return () => clearTimeout(id);
  }, [npIsSpotify, wantPlay, userPaused, npUri, npDurMs, post]);

  // 3c) HANDOFF de device — o Spotify começou a tocar em outro aparelho da conta.
  //     O SDK emitiu takeoverNonce++ (via player_state_changed(null)). Aqui damos
  //     stop na estação (userPaused + callback da página desliga o PLAY mestre) p/
  //     não brigar pelo controle do playback com o outro device. Ref estável do
  //     callback evita re-disparo por mudança de identidade da função.
  const takeoverCb = useRef(onExternalTakeover);
  useEffect(() => {
    takeoverCb.current = onExternalTakeover;
  }, [onExternalTakeover]);
  const lastTakeover = useRef(0);
  useEffect(() => {
    if (status.takeoverNonce === lastTakeover.current) return;
    lastTakeover.current = status.takeoverNonce;
    if (status.takeoverNonce === 0) return; // estado inicial, não é evento
    setUserPaused(true);
    takeoverCb.current?.();
  }, [status.takeoverNonce]);

  /* MIXAGEM CROSS-MOTOR — entrada: começa `uri` AGORA, subindo de 0 até o ganho
     pleno no tempo do ⚙, POR CIMA da faixa que ainda toca no outro motor. Só é
     chamado quando a faixa no ar é de OUTRO provedor (o Spotify tem um único
     playback ativo por conta — dois Spotify nunca se sobrepõem).

     Sem pré-carga possível (o Web Playback SDK não tem `cue`/`preload`): o
     `play()` é justamente o transfer+play, ~1s até sair som. Como começamos
     `fadeMs()` antes do fim da outra faixa, esse atraso cai dentro do trecho em
     que o ganho ainda é quase 0 — inaudível. */
  const startMix = useCallback(
    (uri: string): boolean => {
      if (!uri || !status.ready) return false;
      preStarted.current = uri;
      void (async () => {
        spotVol.current = 0;
        await setVolume(0);
        await play(uri);
        await rampVol(SPOT_TARGET, fadeMs());
      })();
      return true;
    },
    [status.ready, play, setVolume, rampVol],
  );

  const toggle = useCallback(() => {
    if (!npIsSpotify) return;
    // status.playing agora → vai PAUSAR (userPaused=true); senão vai RETOMAR.
    setUserPaused(status.playing);
    void sdkToggle();
  }, [npIsSpotify, status.playing, sdkToggle]);

  // seek por fração (0..1) do waveform → ms. Atualiza a base da interpolação já,
  // p/ o playhead pular na hora sem esperar o evento do SDK. Libera o guard de
  // auto-avanço (seek p/ trás não deve ter disparado "fim").
  const seek = useCallback(
    (frac: number) => {
      if (!npIsSpotify || status.duration <= 0) return;
      const ms = Math.max(0, Math.min(1, frac)) * status.duration;
      base.current = { pos: ms, at: Date.now(), playing: status.playing, uri: npUri };
      setLivePos(ms);
      if (ms < status.duration - 900) advancedFor.current = "";
      void sdkSeek(ms);
    },
    [npIsSpotify, status.duration, status.playing, npUri, sdkSeek],
  );

  /* BLOQUEADO: mandamos tocar e, passados alguns segundos, o SDK segue sem tocar.
     No Spotify não existe `NotAllowedError` pra ler (o comando vai pro device
     remoto e "dá certo"), então o sinal é a AUSÊNCIA de playback. Serve pra UI
     virar o botão do deck em ▶ em vez de ⏹ — que anunciaria "no ar" sem som e, no
     clique natural de quem quer áudio, PARARIA a estação.

     3s: o transfer+play do SDK leva ~1s; abaixo disso acusaria toda faixa nova. */
  const [blocked, setBlocked] = useState(false);
  useEffect(() => {
    if (!npIsSpotify || !wantPlay || userPaused || !npUri) {
      setBlocked(false);
      return;
    }
    if (status.playing) {
      setBlocked(false);
      return;
    }
    const id = setTimeout(() => setBlocked(true), 3000);
    return () => clearTimeout(id);
  }, [npIsSpotify, wantPlay, userPaused, npUri, status.playing]);

  const deck: PlayoutDeck = useMemo(
    () => ({
      isSpotify: npIsSpotify,
      live: npIsSpotify,
      playing: status.playing,
      posMs: livePos,
      durMs: status.duration,
      blocked: npIsSpotify && blocked,
    }),
    [npIsSpotify, status.playing, livePos, status.duration, blocked],
  );

  return { deck, toggle, seek, fadeOut, startMix };
}
