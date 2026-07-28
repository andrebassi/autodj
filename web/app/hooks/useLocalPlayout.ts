"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { NowPlaying } from "@/lib/types";
import { trackAudioURL } from "@/lib/api";
import { bufferTrack, bufferedURL, keepOnly } from "@/lib/audioBuffer";
import { isServerLinked, setAudioAliveProbe } from "@/lib/onAir";
import { fadeEnabled, fadeMs } from "@/lib/fade";
import { resumeFor, saveResume } from "@/lib/resume";
import { instrumentalStatus } from "@/lib/api";
import { toast } from "@/lib/toast";
import { trackGain, targetLUFSFromVol } from "@/lib/gain";
import * as sweep from "@/lib/filterSweep";
import type { MixConfig } from "@/lib/uiContent";
import type { PlayoutDeck } from "@/hooks/useSpotifyPlayout";

/* Intervalo entre consultas do preparo do instrumental, e SÓ enquanto o servidor
   diz que está trabalhando. 10s porque a separação leva minutos: perguntar mais
   rápido não adianta o resultado, e perguntar mais devagar deixa o botão VOZ
   apagado por um tempo perceptível depois de o arquivo já existir. */
const STEM_POLL_MS = 10_000;

/* Tempo de CADA metade da varredura de filtro (sobe o corte, desce o corte) — o
   efeito inteiro é o dobro disto. 750 por lado = 1,5 s de ponta a ponta, afinado de
   ouvido pelo operador: em 420 o gesto passava rápido demais pra ser lido como
   intenção. */
const SWEEP_MS = 750;

/* Fade de saída e de entrada da troca original ⇄ instrumental (VOZ), CADA UM.
   São em SEQUÊNCIA (desce até zero, depois sobe), então o total é o dobro: ~640 ms.
   Ajustado DE OUVIDO pelo operador: 130 ms cada soava apressado — a queda era tão
   rápida que ainda lia como corte, mesmo sem estalo. Aos 320 a descida tem tempo de
   ser percebida COMO fade, que é o que faz a troca soar intencional em vez de falha.
   Ainda é resposta a um clique, então os 3–7s do ⚙ (transição de FAIXA) continuam
   fora de escala aqui. */
const STEM_SWAP_MS = 320;


/* ⚠️ EM SEQUÊNCIA, NUNCA SOBREPOSTAS — e o motivo é o conteúdo.

   Original e instrumental compartilham a base (bateria, baixo, teclado): é a MESMA
   gravação, menos a voz. Sobrepor as duas poria duas cópias dessa base soando
   juntas e, como o decode de cada elemento arranca alguns milissegundos deslocado,
   elas se somariam com defasagem — flanging metálico. Trocaria um artefato por
   outro, e sobrepor QUASE em sync soa pior que não sobrepor (comb filtering é mais
   audível justamente entre 1 e 20 ms de defasagem).

   Sync de amostra resolveria, mas exige Web Audio com os dois arquivos decodificados
   em memória (~85 MB por versão numa música de 4 min) e traria o AudioContext de
   volta ao playout — o que a 0.2.32 removeu. Decisão do operador: fade out e in
   bem rápido, que é o que está aqui.

// Fade de transição (PRÓXIMA/RETROCEDER): a faixa nova sobe e a que sai desce, no
// lugar do corte seco. Alvo = 1.0 (volume pleno do <audio>).
//
// A DURAÇÃO vem de `fadeMs()` (⚙ Configurações: 0/3/5/7s), lida NA HORA da
// transição — não em constante. Este motor tinha 3000 cravado enquanto Spotify e
// YouTube já liam a configuração: quem escolhia 7s via os streamings respeitarem e
// as faixas locais insistirem em 3s, sem explicação. `fadeEnabled()` (0 = corte
// seco) já era respeitado; só a duração ficava presa.

/* Driver de playout LOCAL (PCM) no browser — espelha o useSpotifyPlayout, mas p/
   faixas locais (source != spotify). Sem SDK/DRM: toca via <audio> apontando pra
   /library/tracks/{id}/audio (suporta Range/seek).

   A fila é a agenda; quando o now-playing é LOCAL, este hook:
   1. carrega e toca o arquivo no <audio> (som de verdade no browser);
   2. alimenta posMs/durMs reais (currentTime/duration) → o Deck A segue a música;
   3. no fim (`ended`) chama POST /playback/next → a fila avança A→B sozinha.

   Antes NÃO existia driver de avanço p/ faixa local (só o Spotify avançava pelo
   evento do SDK): Deck A terminava e Deck B nunca entrava (bug reportado). Este
   hook fecha esse buraco. Fallback: se o arquivo não carregar (sem PCM/permissão/
   arquivo ausente), um timer pela duração conhecida avança mesmo assim — a agenda
   nunca trava. Guard `advancedFor` garante 1 avanço por faixa (evento OU timer).

   PRÉ-CARGA (v0.2.11): são DOIS <audio> em ping-pong. Um toca a faixa no ar; o
   outro já tem a PRÓXIMA da fila com `src` setado e `preload="auto"`, então o
   arquivo é baixado ANTES de entrar no ar. Na virada só trocamos o papel (o
   elemento pré-carregado já está em 00:00, pronto) — sem request novo, sem
   espera de rede no começo da faixa. Sem a próxima pré-carregada (fila mudou),
   cai no caminho antigo: seta o `src` no elemento que já está no ar.

   MIXAGEM LOCAL↔LOCAL (v0.2.26) — `startMix`. Até aqui só Spotify↔YouTube
   sobrepunha (v0.2.12); entre duas faixas locais a virada era sequencial (desce,
   troca, sobe) — buraco no meio, justamente no caso mais comum de quem toca o
   próprio acervo. Agora o mesmo coordenador (`useCrossEngineMix`) chama o
   `startMix` DESTE hook: o elemento ocioso — que já está bufferizado pela
   pré-carga — começa a tocar mudo e sobe, enquanto o do ar desce. Dois `<audio>`
   soando juntos = crossfade de verdade, o que Spotify e YouTube nunca conseguem
   entre si na mesma conta.
   **A fila NÃO é adiantada** (mesma regra da v0.2.12): o servidor avança só no fim
   natural, e quando o now-playing alcança a entrante o handoff a **ADOTA**
   (`preStarted`) em vez de recomeçá-la do 00:00 — sem isso a faixa voltaria uns
   segundos e o fade-in dispararia de novo, no meio da música. */
export function useLocalPlayout(
  snap: NowPlaying | null,
  post: (path: string) => void,
  wantPlay = true,
  // Configuração de áudio das faixas locais (⚙ Configurações): trim por tipo de
  // mídia, normalização de ganho e alvo. `undefined` = ainda carregando ⇒ tudo
  // desligado, que é o comportamento anterior — nunca aplicar meia-config.
  mix?: MixConfig,
  // LOOP armado pelo operador: enquanto ligado, a faixa no ar volta ao começo do
  // áudio ao alcançar a entrada do vocal, segurando a base instrumental.
  loopArmed = false,
  // MODO VOZ (🚫🎤): enquanto ligado, a faixa local toca pelo INSTRUMENTAL
  // separado por IA (`?stem=instrumental`) em vez do arquivo original. Só liga em
  // faixa que já tem o instrumental preparado — sem preparo não há o que tocar.
  // Não é persistido — F5 volta desligado, igual ao LOOP.
  vocalCut = false,
): {
  deck: PlayoutDeck;
  toggle: () => void;
  seek: (frac: number) => void;
  fadeOut: () => Promise<void>;
  /** põe a PRÓXIMA local no ar agora, subindo de 0 (mixagem local↔local) */
  startMix: (uri: string) => boolean;
} {
  const np = snap?.now_playing;
  // catch-all local = tudo que NÃO é out-of-band (Spotify SDK / YouTube IFrame).
  // Sem excluir youtube, o motor local disputaria a faixa YouTube (dois drivers
  // no mesmo now-playing) → avanço duplo. Espelho: useYouTubePlayout só pega
  // source==="youtube"; aqui pegamos o resto.
  const isLocal =
    !!np && np.source !== "spotify" && np.source !== "youtube" && !!np.track_id;
  const trackId = isLocal ? np!.track_id : "";
  // a RETOMADA fala por uri (linguagem comum aos três motores); este hook trabalha
  // por track_id. Pra faixa local a uri é o caminho do arquivo.
  const npUri = isLocal ? (np?.uri ?? "") : "";
  const durMs = np?.duration_ms ?? 0;
  // próxima da fila (queue[0] = no ar, queue[1] = próxima) → alvo da pré-carga.
  const nextPlayable = snap?.queue?.[1]?.playable;
  const nextTrackId =
    nextPlayable &&
    nextPlayable.source !== "spotify" &&
    nextPlayable.source !== "youtube" &&
    nextPlayable.track_id
      ? nextPlayable.track_id
      : "";
  // uri da próxima: o coordenador da mixagem fala por URI (linguagem comum aos 3
  // motores), este hook trabalha por track_id — o par abaixo é a tradução.
  const nextUri = nextPlayable?.uri ?? "";
  const mixTargetRef = useRef({ uri: "", trackId: "" });
  mixTargetRef.current = { uri: nextUri, trackId: nextTrackId };
  /* As 3 PRÓXIMAS locais da fila — alvo do buffer anti-queda. Três porque cobre
     ~12 min de fila por ~10 MB; a virada em si só precisa de uma, mas uma queda
     de internet dura mais que uma faixa. `join` na dep pra não re-disparar o
     efeito a cada snapshot do WS (a fila reemite igual a cada 2s). */
  /* GANHO DA FAIXA NO AR. Substitui o literal `1` que significava "volume pleno"
     em 7 pontos deste hook — carregar, tocar, rampar, retomar, adotar. Todos
     passam a mirar ESTE valor, exatamente como o motor do Spotify já mira o seu
     `SPOT_TARGET = 0.8`.

     Fica num ref (e não só numa const) porque os callbacks de ramp/toggle rodam
     fora do ciclo de render e leriam um valor velho — o mesmo motivo do
     `liveIdxRef`. */
  const normalize = !!mix?.normalize;
  const targetLUFS = targetLUFSFromVol(mix?.normalizeVol ?? 0);
  const gain = trackGain({
    loudnessLUFS: np?.loudness_lufs ?? 0,
    targetLUFS,
    normalize,
  });
  const gainRef = useRef(1);
  gainRef.current = isLocal ? gain : 1;
  // ganho da PRÓXIMA: é o alvo da rampa da mixagem. Sem ele a entrante subiria
  // até 1 e o salto de volume apareceria justamente na virada, que é onde mais
  // incomoda — o oposto do que a normalização existe pra resolver.
  const nextGainRef = useRef(1);
  nextGainRef.current = trackGain({
    loudnessLUFS: nextPlayable?.loudness_lufs ?? 0,
    targetLUFS,
    normalize,
  });

  /* TRIM. Cada tipo de mídia tem seu interruptor no ⚙ (música e vinheta são
     decisões diferentes: cortar o silêncio de uma vinheta muda o timing dela). */
  const trimOn = (() => {
    if (!mix) return false;
    const cat = np?.category ?? "";
    // "jingle" é a categoria de vinheta no domínio; o resto cai na régua de música.
    return cat === "jingle" ? mix.trimVinhetas : mix.trimMusicas;
  })();
  const cueInMs = trimOn ? (np?.cue_in_ms ?? 0) : 0;
  const rawCueOut = trimOn ? (np?.cue_out_ms ?? 0) : 0;
  // vocal só interessa com marca de verdade; 0 = "não marcado" (regra do operador).
  const vocalMs = isLocal ? (np?.vocal_ms ?? 0) : 0;
  /* INSTRUMENTAL PRONTO (separação por IA) — consultado AO VIVO, não lido do
     `Playable`.

     O Playable é um retrato do instante do enfileiramento, e a separação termina
     MINUTOS depois: o retrato diria "não" para sempre. Por isso o estado vem do
     endpoint.

     É a ÚNICA régua do modo VOZ desde a 0.2.32: o cancelamento de centro foi
     aposentado (tirava 2 dB de voz em música real), então "dá pra tirar o vocal"
     virou exatamente "o instrumental está pronto". */
  const [hasStem, setHasStem] = useState(false);
  useEffect(() => {
    setHasStem(false);
    // NÃO gateado no `vocalCut`: o botão VOZ precisa saber disto pra poder LIGAR.
    if (!isLocal || !trackId) return;
    let stop = false;
    let timer = 0;
    /* PERGUNTA DE NOVO ENQUANTO HÁ TRABALHO EM CURSO.

       Uma consulta só por faixa não basta, e o operador sentiu isso: a separação
       leva minutos, então ela costuma TERMINAR com a faixa já no ar. Com a
       resposta congelada em "queued", o botão VOZ ficava apagado até a faixa
       trocar — o preparo tinha acabado e a tela não sabia.

       O poll só existe ENQUANTO o servidor diz que está trabalhando: `ready` e
       `failed` são finais e encerram o ciclo. Sem trabalho pendente não há
       requisição nenhuma — o mesmo princípio do `useInstrumental`. */
    const ask = () => {
      void instrumentalStatus(trackId)
        .then((r) => {
          if (stop || !r) return;
          if (r.status === "ready") {
            setHasStem(true);
            return; // final: para de perguntar
          }
          if (r.status === "queued" || r.status === "running") {
            timer = window.setTimeout(ask, STEM_POLL_MS);
          }
        })
        .catch(() => {
          /* sem servidor / rota ausente: o modo simplesmente não habilita */
        });
    };
    ask();
    return () => {
      stop = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [isLocal, trackId]);
  // refs: os efeitos de carga leem isto fora do ciclo de render.
  const hasStemRef = useRef(false);
  hasStemRef.current = hasStem;
  const vocalCutRef = useRef(false);
  vocalCutRef.current = vocalCut;

  // rastreia qual fonte (original vs instrumental) está no elemento no ar. O
  // efeito que faz a troca mora mais abaixo, depois de `liveAudio` existir.
  const stemSrcRef = useRef<boolean | null>(null);
  /* troca de fonte em curso: o `pause` do elemento que sai não é uma pausa da
     ESTAÇÃO, e não pode virar RETOMAR no transporte.

     Também é o que impede a troca e o CROSSFADE de brigarem: os dois usam o
     elemento ocioso, e a troca o segura por segundos. `startMix` recusa enquanto
     isto for true — melhor uma virada seca que dois donos do mesmo elemento. */
  const swappingRef = useRef(false);
  // posição/fim espelhados em ref: a troca precisa saber se a faixa está
  // acabando, e pôr `posMs` nas deps do efeito o faria re-rodar a cada tick.
  const posRef = useRef(0);
  const endRef = useRef(0);

  /* Estas dicas vivem em REF, não nas deps dos efeitos, de propósito: mexer num
     interruptor do ⚙ NÃO pode reposicionar a música que já está no ar. A troca
     vale a partir da PRÓXIMA faixa — cortar o áudio sob os pés do operador seria
     pior que esperar a virada. */
  const cueInRef = useRef(0);
  cueInRef.current = cueInMs;
  const cueOutRef = useRef(0);
  cueOutRef.current = rawCueOut;
  const vocalRef = useRef(0);
  vocalRef.current = vocalMs;
  // LOOP, por outro lado, é ação IMEDIATA do operador (ele armou agora para falar
  // agora) — por isso o ref é lido no tick, que roda a cada `timeupdate`.
  const loopRef = useRef(false);
  loopRef.current = loopArmed && isLocal && vocalMs > 0;
  // cue de entrada da PRÓXIMA — a mixagem precisa dele (senão o crossfade
  // sobreporia o vazio da frente da faixa que entra).
  const nextCueInRef = useRef(0);
  nextCueInRef.current = (() => {
    if (!mix || !nextPlayable) return 0;
    const cat = nextPlayable.category ?? "";
    const on = cat === "jingle" ? mix.trimVinhetas : mix.trimMusicas;
    return on ? (nextPlayable.cue_in_ms ?? 0) : 0;
  })();

  const bufferIds = (snap?.queue ?? [])
    .slice(1, 4)
    .map((q) => q.playable)
    .filter((p) => p && p.source !== "spotify" && p.source !== "youtube" && p.track_id)
    .map((p) => p!.track_id);
  const bufferKey = bufferIds.join(",");

  // dois elementos em ping-pong: [0] e [1]. liveIdx diz qual está no ar; o outro
  // carrega a próxima. Ref espelha o índice p/ os callbacks (fade, toggle, seek)
  // acertarem o elemento no ar mesmo fora do ciclo de render.
  const elemsRef = useRef<(HTMLAudioElement | null)[]>([null, null]);
  const [liveIdx, setLiveIdx] = useState<0 | 1>(0);
  const liveIdxRef = useRef<0 | 1>(0);
  liveIdxRef.current = liveIdx;
  const liveAudio = useCallback(
    (): HTMLAudioElement | null => elemsRef.current[liveIdxRef.current],
    [],
  );
  const idleAudio = useCallback(
    (): HTMLAudioElement | null => elemsRef.current[liveIdxRef.current === 0 ? 1 : 0],
    [],
  );
  // faixa já pré-carregada no elemento ocioso ("" = nenhuma).
  const cuedTrack = useRef<string>("");
  const [playing, setPlaying] = useState(false);
  // browser recusou o autoplay: a estação QUER tocar mas não sai som até um gesto.
  const [blocked, setBlocked] = useState(false);
  const [posMs, setPosMs] = useState(0);
  const [realDur, setRealDur] = useState(0);
  const advancedFor = useRef<string>("");
  const lastLoaded = useRef<string>("");
  // posição a aplicar assim que o elemento souber a duração (retomada pós-refresh).
  const pendingSeek = useRef(0);
  // de onde esta faixa COMEÇOU a tocar nesta sessão (0 normalmente, >0 se retomada).
  const startedAtMs = useRef(0);
  // PAUSE do deck = intent do operador de SEGURAR a faixa. Distinto do wantPlay
  // (gate mestre da estação): pausar não para a estação, mas DEVE congelar o
  // auto-avanço. Sem isso o fallback timer por duração (wall-clock, só olhava
  // wantPlay) disparava /playback/next mesmo pausado → avançava e o load da
  // próxima re-tocava, "tirando" o pause. Reseta a cada troca de faixa.
  const [userPaused, setUserPaused] = useState(false);

  // ramp de volume do <audio> (fade). Cancela ramp em voo. fadeInPending marca que
  // a faixa carregada deve subir do 0 ao dar play (só faixa nova, não retomada).
  // DOIS timers, um por elemento: na mixagem os dois rampam AO MESMO TEMPO em
  // direções opostas. Com um timer só, a segunda rampa cancelaria a primeira e um
  // dos lados ficaria congelado no volume em que estava.
  const fadeTimer = useRef<number | null>(null);
  const mixTimer = useRef<number | null>(null);
  const fadeInPending = useRef(false);
  // faixa que já está tocando "por fora" (mixagem em curso) — vira adoção quando o
  // now-playing do servidor finalmente a alcança.
  const preStarted = useRef<string>("");
  const clearTimer = useCallback((t: { current: number | null }) => {
    if (t.current !== null) {
      clearInterval(t.current);
      t.current = null;
    }
  }, []);
  const clearFade = useCallback(() => clearTimer(fadeTimer), [clearTimer]);
  const rampEl = useCallback(
    (
      el: HTMLAudioElement | null,
      to: number,
      ms: number,
      timer: { current: number | null },
    ): Promise<void> =>
      new Promise((resolve) => {
        if (!el) {
          resolve();
          return;
        }
        clearTimer(timer);
        const from = el.volume;
        if (ms <= 0 || Math.abs(to - from) < 0.001) {
          el.volume = Math.max(0, Math.min(1, to));
          resolve();
          return;
        }
        /* ⚠️ O PASSO É ADAPTATIVO, E ISSO É O QUE MATA O ESTALO.

           Era 50 ms fixo. Num fade de 3–7 s dá 60–140 passos e ninguém percebe;
           numa rampa curta (a troca do VOZ, 140 ms) dá TRÊS passos, ou seja, o
           volume pula 1,0 → 0,67 → 0,33 → 0 — saltos de ~3,5 dB. Salto de ganho é
           descontinuidade na forma de onda, e descontinuidade É clique (zipper
           noise). O "repique" da troca vinha daqui, não do arquivo trocando.

           ~30 passos é o bastante para o ouvido ler como contínuo; o piso de 8 ms
           evita disparar timer mais rápido do que o browser entrega. */
        const stepMs = ms <= 600 ? Math.max(8, Math.round(ms / 30)) : 50;
        const steps = Math.max(1, Math.round(ms / stepMs));
        let i = 0;
        timer.current = window.setInterval(() => {
          i++;
          /* Curva em cosseno (smoothstep), não reta: a rampa linear chega e sai
             com quina — a derivada muda de golpe nas pontas, e isso também estala.
             O cosseno entra e sai devagar, então as pontas não têm quina. */
          const t = i / steps;
          const eased = (1 - Math.cos(Math.PI * t)) / 2;
          el.volume = Math.max(0, Math.min(1, from + (to - from) * eased));
          if (i >= steps) {
            clearTimer(timer);
            el.volume = Math.max(0, Math.min(1, to));
            resolve();
          }
        }, stepMs);
      }),
    [clearTimer],
  );
  const rampVolume = useCallback(
    (to: number, ms: number): Promise<void> => rampEl(liveAudio(), to, ms, fadeTimer),
    [rampEl, liveAudio],
  );
  useEffect(
    () => () => {
      clearTimer(fadeTimer);
      clearTimer(mixTimer);
    },
    [clearTimer],
  );

  /* MIXAGEM: começa a PRÓXIMA local agora, muda, subindo até o volume pleno na
     duração do ⚙. Chamado pelo coordenador `useCrossEngineMix` a `fadeMs()` do fim
     natural — a que está no ar desce em paralelo (`fadeOut`).

     Devolve `false` sem tocar em nada quando não dá pra sobrepor de verdade: sem
     pré-carga, elemento ainda sem dados (`readyState < 2`) ou pedido de outra uri.
     O coordenador trata `false` como "não mixa" e NÃO baixa o volume do que está no
     ar — melhor uma virada seca que um silêncio. */
  const startMix = useCallback(
    (uri: string): boolean => {
      const idle = idleAudio();
      const cur = liveAudio();
      const target = mixTargetRef.current;
      if (!idle || !cur || !target.trackId) return false;
      /* ⚠️ TROCA DE FONTE EM CURSO ⇒ NÃO MIXA. Os dois usam o elemento ocioso: a
         troca do SEM-VOZ o toma por segundos, e deixar a mixagem entrar por cima
         resultava em dois donos do mesmo elemento — o crossfade sobrescrevia a
         troca no meio, os papéis não fechavam e o deck ficava anunciando "no ar"
         com volume zero. Recusar aqui devolve `false`, e o coordenador faz a
         virada seca sem baixar o volume do que está tocando. */
      if (swappingRef.current) return false;
      if (target.uri !== uri) return false; // fila mudou entre o gatilho e aqui
      if (cuedTrack.current !== target.trackId || !idle.src) return false; // sem pré-carga
      if (idle.readyState < 2) return false; // ainda sem dados: tocaria buraco
      try {
        // a entrante também começa no cue dela: sem isto a mixagem sobreporia o
        // VAZIO da frente da próxima faixa, ou seja, o crossfade cairia no
        // silêncio — o mesmo buraco que o trim existe pra eliminar.
        idle.currentTime = nextCueInRef.current / 1000;
      } catch {
        /* sem metadata ainda — começa do 0 de qualquer forma */
      }
      idle.volume = 0;
      preStarted.current = target.trackId;
      void idle.play().catch(() => {
        // não conseguiu arrancar (autoplay/decode): desfaz a mixagem e devolve o
        // ganho pleno pra faixa no ar, senão ela terminaria de descer no vazio.
        preStarted.current = "";
        clearTimer(mixTimer);
        idle.pause();
        clearFade();
        cur.volume = gainRef.current;
      });
      // a entrante sobe até o ganho DELA (calculado pelo loudness dela), não até
      // 1 — senão a normalização se perderia exatamente na virada, que é onde o
      // salto de volume mais incomoda.
      void rampEl(idle, nextGainRef.current, fadeMs(), mixTimer);
      return true;
    },
    [idleAudio, liveAudio, rampEl, clearTimer, clearFade],
  );

  /* Sonda de "tem som saindo?" — quem pergunta é o hook de conectividade, pra não
     recarregar a página quando a faixa local atravessou a queda tocando.
     `currentTime > 0` junto com `!paused` evita o falso positivo do elemento que
     aceitou o play e está com o carregamento adiado (o bloqueio de autoplay do
     Chrome deixa exatamente esse estado). */
  useEffect(() => {
    setAudioAliveProbe(() => {
      const a = liveAudio();
      return !!a && !a.paused && a.currentTime > 0 && !a.ended;
    });
    return () => setAudioAliveProbe(null);
  }, [liveAudio]);

  // os dois <audio> criados uma vez (client-only; nunca no prerender).
  // preload="auto" é o que faz o elemento ocioso baixar a próxima faixa sozinho.
  useEffect(() => {
    if (typeof window === "undefined") return;
    const a0 = new Audio();
    const a1 = new Audio();
    a0.preload = "auto";
    a1.preload = "auto";
    elemsRef.current = [a0, a1];
    return () => {
      for (const a of [a0, a1]) {
        a.pause();
        a.src = "";
      }
      elemsRef.current = [null, null];
    };
  }, []);

  /* CONTEXTO DE ÁUDIO — só para SINTETIZAR o efeito da troca (ver `lib/swapFx`).

     ⚠️ Ele NÃO roteia o `<audio>`. É a diferença que torna isto seguro depois de a
     0.2.32 ter tirado Web Audio do playout: nenhum elemento passa por
     `createMediaElementSource`, então nem um contexto suspenso nem um erro aqui
     podem deixar a estação muda. O pior caso é o efeito não soar.

     Criado só na primeira troca, que é um clique — nunca no carregamento. */
  const fxCtxRef = useRef<AudioContext | null>(null);
  const fxContext = useCallback((): AudioContext | null => {
    const Ctor =
      window.AudioContext ??
      (window as unknown as { webkitAudioContext?: typeof AudioContext })
        .webkitAudioContext;
    if (!Ctor) return null;
    if (!fxCtxRef.current) {
      try {
        fxCtxRef.current = new Ctor();
      } catch {
        return null;
      }
    }
    const ctx = fxCtxRef.current;
    if (ctx.state === "suspended") void ctx.resume();
    return ctx;
  }, []);
  useEffect(
    () => () => {
      void fxCtxRef.current?.close().catch(() => {});
      fxCtxRef.current = null;
    },
    [],
  );

  /* TROCA DE FONTE quando o operador liga/desliga o VOZ numa faixa que TEM
     instrumental pronto — original ⇄ arquivo separado.

     ⚠️ TROCAR O `src` DO ELEMENTO NO AR DÁ REPIQUE, e foi o que o operador ouviu.
     Não é ruído de fade: entre o `src = ...` e o `play()` na posição certa o
     browser descarrega o buffer, faz request novo, decodifica e busca o ponto —
     dezenas a centenas de milissegundos de nada, com um estalo nas bordas do
     corte. Nenhum ajuste de volume no MESMO elemento resolve, porque o elemento
     em si para de produzir áudio.

     O que resolve é o mesmo mecanismo do crossfade local↔local (0.2.26): a fonte
     nova entra no elemento OCIOSO, já tocando e já na posição, e só então os dois
     volumes se cruzam. Em nenhum instante existe silêncio — um lado sempre está
     soando. É crossfade de verdade, não um curativo por cima do corte.

     Ao final os PAPÉIS TROCAM (mesmo padrão do handoff de faixa) e `cuedTrack` é
     zerado, para o efeito de pré-carga recarregar a próxima no elemento que
     sobrou. O custo é rebaixar a próxima faixa — barato e raro, contra um estalo
     no ar toda vez que o operador aperta o botão. */
  useEffect(() => {
    const a = liveAudio();
    const idle = idleAudio();
    if (!a || !isLocal || !trackId || !hasStem) return;
    const want = vocalCut;
    /* ⚠️ `null` = NÃO SEI o que está no elemento, e "não sei" NÃO pode virar uma
       troca. Este ref só é null antes de o efeito de carga rodar; quem carrega é
       que sabe qual fonte escolheu, e é ele que anota.

       Sem esta guarda o bug era grave e invisível até o acervo ter instrumentais:
       ao terminar a consulta, `hasStem` virava true e disparava este efeito com
       `want = false` (VOZ desligado) contra `stemSrcRef = null`. Como null !== false,
       ele "trocava" a fonte do original PARA O ORIGINAL — um `src =` novo no
       elemento no ar, ou seja, reload no meio da música. Foi o que o operador
       ouviu como corte seco a cada faixa. */
    if (stemSrcRef.current === null) return;
    if (stemSrcRef.current === want) return;
    stemSrcRef.current = want;
    if (!a.src) return; // ainda não carregou: o efeito de carga já escolhe certo
    const at = a.currentTime;
    const wasPlaying = !a.paused;

    /* Pausado, ou sem elemento ocioso disponível, não há o que cruzar: troca
       seca mesmo. Sem áudio no ar não existe repique a esconder. */
    if (!idle || !wasPlaying) {
      a.src = trackAudioURL(trackId, want);
      const restore = () => {
        try {
          a.currentTime = at;
        } catch {
          /* sem metadata: começa do 0, e a barra corrige no próximo tick */
        }
        if (wasPlaying) void a.play().catch(() => {});
      };
      a.addEventListener("loadedmetadata", restore, { once: true });
      return;
    }

    /* ⚠️ NÃO TROCA PERTO DO FIM — foi o que quebrou a mixagem em produção.

       A varredura leva 6 s. Se a faixa acaba no meio dela, o efeito é abortado
       pela troca de faixa, `finishSwap()` não roda e os papéis ficam PELA METADE:
       o elemento que o app considera "ao vivo" está em volume 0 (a rampa desceu)
       enquanto quem toca é o outro. O deck anuncia "no ar", o transporte mostra
       RETOMAR, o tempo congela — e a mixagem nem tenta, porque o coordenador tem
       um gate em `playing`.

       Perto do fim a troca é dispensável: a PRÓXIMA faixa já carrega na fonte
       certa (o efeito de carga lê `vocalCut`+`hasStem`). Só registramos a
       intenção e saímos. */
    const remaining = endRef.current - posRef.current;
    const gesture = SWEEP_MS * 2 + 1500;
    if (remaining > 0 && remaining < gesture) {
      return;
    }

    let aborted = false;
    let finished = false;
    swappingRef.current = true;
    /* O contexto é pedido AQUI, no começo, e não na hora de filtrar: `resume()` é
       assíncrono e `attach` recusa contexto que não esteja `running`. Pedindo cedo,
       ele sobe durante a carga do arquivo; pedindo tarde, a PRIMEIRA troca da
       sessão sairia sem efeito e o operador veria o botão se comportar diferente
       na primeira vez. */
    const ctx = fxContext();
    // guardadas p/ o cleanup: elemento roteado e esquecido no topo da varredura
    // continuaria sem grave na faixa seguinte.
    const used: sweep.SweepChain[] = [];
    idle.pause();
    idle.volume = 0;
    idle.src = trackAudioURL(trackId, want);
    cuedTrack.current = ""; // o ocioso deixou de guardar a próxima
    /* `at` é a posição de quando o botão foi apertado; carregar leva tempo, então
       o alvo é recalculado do elemento no ar no instante em que o novo está
       pronto — senão a troca voltaria alguns décimos toda vez. */
    /* troca de papéis — o mesmo fim para o caminho com filtro e para o degradado. */
    const finishSwap = () => {
      finished = true;
      a.volume = gainRef.current; // devolve o ganho: este elemento vai pré-carregar
      setRealDur(idle.duration && isFinite(idle.duration) ? idle.duration * 1000 : 0);
      const nidx: 0 | 1 = liveIdxRef.current === 0 ? 1 : 0;
      liveIdxRef.current = nidx; // antes do setState: fade/toggle já miram o novo
      setLiveIdx(nidx);

      /* ⚠️ AFIRMA o estado a partir de quem ASSUMIU, em vez de esperar evento.

         `playing` só muda por `play`/`pause` do elemento ao vivo. O que assume já
         estava tocando enquanto era o ocioso, então NENHUM evento novo dispara
         depois da virada de papéis — sem esta linha o estado fica o que era, e um
         `false` acidental nunca mais é desfeito. Mesmo padrão da adoção da
         mixagem, que também semeia estado no handoff. */
      setPlaying(!idle.paused);

      /* ⚠️ E O GUARDA SÓ CAI NO PRÓXIMO CICLO — foi ISTO que matava a mixagem.

         `a.pause()` não emite o evento na hora: o browser o AGENDA. Liberando o
         guarda aqui, de forma síncrona, o evento chegava depois e passava direto,
         marcando a estação como parada no meio de uma música que seguia tocando.
         Como o coordenador da mixagem tem gate em `playing`, o crossfade parava de
         acontecer — e nada trazia o estado de volta até a faixa virar.

         Medido: 107 de 111 amostras com o transporte em RETOMAR e ZERO amostras em
         mixagem, com o áudio tocando normalmente. */
      window.setTimeout(() => {
        swappingRef.current = false;
      }, 0);
    };

    const start = () => {
      if (aborted) return;
      try {
        idle.currentTime = a.currentTime;
      } catch {
        idle.currentTime = at;
      }
      void idle
        .play()
        .then(() => {
          if (aborted) return;
          /* VARREDURA DE FILTRO + CROSSFADE, os dois no mesmo gesto.

             O grave sai da faixa no ar (corte subindo até 1,4 kHz) e volta pela
             faixa que entrou — aos ouvidos é UMA varredura só atravessando a
             troca. Por baixo dela, a voz vai embora e o instrumental chega ao
             longo do gesto INTEIRO, não num corte no meio.

             Por que a sobreposição agora é segura, se antes eu a recusei: o
             problema era comb filtering (as duas cópias da MESMA base somadas com
             milissegundos de defasagem). O filtro resolve isso de graça — no
             miolo do cruzamento os dois estão passa-alta em 1,4 kHz, ou seja, sem
             o grave e o médio-grave que são justamente o conteúdo correlacionado
             que produzia a lavagem. Cruzar embaixo de um filtro é diferente de
             cruzar em banda cheia.

             Sem filtro (contexto indisponível) cai no fade sequencial de sempre:
             o efeito nunca pode ser condição pra a troca acontecer. */
          const outChain = ctx ? sweep.attach(a, ctx) : null;
          const inChain = ctx ? sweep.attach(idle, ctx) : null;
          if (outChain) used.push(outChain);
          if (inChain) used.push(inChain);
          const filtered = !!outChain && !!inChain;

          if (!filtered) {
            // caminho degradado: desce, troca, sobe (sem sobreposição, porque sem
            // filtro a sobreposição volta a lavar).
            void rampEl(a, 0, STEM_SWAP_MS, fadeTimer)
              .then(() => {
                if (aborted) return;
                a.pause();
                return rampEl(idle, gainRef.current, STEM_SWAP_MS, mixTimer);
              })
              .then(() => {
                if (!aborted) finishSwap();
              });
            return;
          }

          sweep.parkTop(inChain!); // entra já sem grave, pra varredura ser contínua

          /* As duas rampas de volume correm pelo gesto INTEIRO (2 × SWEEP_MS) e
             começam JUNTAS — é isso que faz "a voz ir sumindo" em vez de sumir de
             uma vez no topo. O filtro, esse sim, é assimétrico: sobe pela que sai,
             desce pela que entra. */
          const whole = SWEEP_MS * 2;
          void rampEl(a, 0, whole, fadeTimer);
          void rampEl(idle, gainRef.current, whole, mixTimer);

          void sweep
            .sweepUp(outChain!, SWEEP_MS)
            .then(() => {
              if (aborted) return;
              /* O grave volta PELA faixa que entrou. A que sai fica no topo de
                 propósito pelo resto do gesto: mantê-la fina evita que o grave
                 dela e o da entrante se somem enquanto ainda há um resto de
                 volume nela. */
              return sweep.sweepDown(inChain!, SWEEP_MS);
            })
            .then(() => {
              if (aborted) return;
              a.pause(); // chegou em zero pela rampa longa: silencioso
              sweep.reset(outChain!); // volta ao repouso: vai pré-carregar
              finishSwap();
            });
        })
        .catch(() => {
          /* o browser recusou: fica no que já está no ar, sem cortar nada.
             Solta o guarda — deixá-lo preso engoliria uma pausa REAL depois. */
          swappingRef.current = false;
          stemSrcRef.current = !want;
        });
    };
    if (idle.readyState >= 1) start();
    else idle.addEventListener("loadedmetadata", start, { once: true });
    return () => {
      aborted = true;
      swappingRef.current = false;
      // varredura interrompida no meio (troca de faixa, novo clique): devolve o
      // grave na hora, senão a próxima faixa entra fina e ninguém sabe por quê.
      for (const c of used) sweep.reset(c);
      /* ⚠️ E DESFAZ A TROCA PELA METADE. Sem isto, uma interrupção no meio dos 6 s
         deixava o elemento "ao vivo" em volume 0 e o outro tocando — estado
         inconsistente que travava a mixagem e congelava o relógio do deck.
         Reverter é mais seguro que completar: quem estava no ar continua no ar, e
         a próxima faixa carrega na fonte certa de qualquer forma. */
      if (!finished) {
        clearTimer(fadeTimer);
        clearTimer(mixTimer);
        const live = liveAudio();
        const other = idleAudio();
        if (live) live.volume = gainRef.current;
        if (other) {
          other.pause();
          other.volume = 0;
        }
        stemSrcRef.current = null; // "não sei mais o que está carregado"
      }
    };
  }, [
    vocalCut,
    hasStem,
    isLocal,
    trackId,
    liveAudio,
    idleAudio,
    rampEl,
    mixTimer,
    fadeTimer,
    fxContext,
    clearTimer,
  ]);
  // troca de faixa zera o rastreio de qual fonte está no elemento.
  useEffect(() => {
    stemSrcRef.current = null;
  }, [trackId]);

  // now-playing local mudou → só CARREGA a faixa no <audio> (src + reset). NÃO
  // dá play aqui: quem arranca/pausa é o effect do wantPlay abaixo. Assim a faixa
  // fica engatilhada a 00:00 (nada no ar / AUTO OFF esperando operador) até darem
  // PLAY — comportamento idêntico ao Spotify (hexagonal: recurso igual p/ todo
  // provider). lastLoaded evita re-disparo a cada snapshot do WS.
  useEffect(() => {
    const a = liveAudio();
    if (!a) return;
    // ABORTO da mixagem: o now-playing virou OUTRA coisa que não a faixa
    // pré-iniciada (pulo manual no meio da janela, ou entrou Spotify/YouTube). O
    // elemento que estava tocando por fora precisa calar — senão fica um áudio
    // órfão soando por cima do que o operador escolheu.
    if (preStarted.current && (!isLocal || preStarted.current !== trackId)) {
      const orphan = idleAudio();
      clearTimer(mixTimer);
      if (orphan) {
        orphan.pause();
        orphan.volume = 0;
        try {
          orphan.currentTime = 0;
        } catch {
          /* sem metadata — irrelevante, está mudo e pausado */
        }
      }
      preStarted.current = "";
    }
    if (!isLocal) {
      a.pause();
      setPlaying(false);
      setPosMs(0);
      setRealDur(0);
      setUserPaused(false);
      lastLoaded.current = "";
      return;
    }
    if (trackId === lastLoaded.current) return;
    lastLoaded.current = trackId;
    advancedFor.current = "";
    setUserPaused(false); // faixa nova entra fresca (segue o wantPlay)
    setPosMs(0);
    // faixa nova entra muda (volume 0) e sobe no play (fade-in de 3s) — sem corte
    // seco. clearFade cancela qualquer ramp da faixa anterior ainda em voo. Fade
    // desligado (⚙ Configurações) → entra a volume pleno (corte seco).
    clearFade();
    const vol = fadeEnabled() ? 0 : gainRef.current;
    fadeInPending.current = fadeEnabled();

    const idle = idleAudio();

    // ADOÇÃO (mixagem): a faixa que o servidor acabou de promover JÁ ESTÁ TOCANDO
    // — o `startMix` a arrancou uns segundos atrás. Aqui só trocamos os papéis e
    // deixamos ela em paz: mexer em `currentTime` a jogaria pro 00:00 no meio da
    // música, e re-armar o fade-in a faria cair pra 0 e subir de novo.
    if (idle && preStarted.current === trackId) {
      // a pré-partida da mixagem carregou o ORIGINAL (a pré-carga não conhece
      // instrumental); com o VOZ ligado, o efeito de troca cruza pro separado.
      stemSrcRef.current = false;
      a.pause();
      a.volume = gainRef.current; // devolve o ganho: este elemento vai pré-carregar
      preStarted.current = "";
      cuedTrack.current = "";
      fadeInPending.current = false;
      setRealDur(idle.duration && isFinite(idle.duration) ? idle.duration * 1000 : 0);
      setPosMs(idle.currentTime * 1000);
      setPlaying(!idle.paused);
      const nidx: 0 | 1 = liveIdxRef.current === 0 ? 1 : 0;
      liveIdxRef.current = nidx;
      setLiveIdx(nidx);
      return;
    }

    // HANDOFF: a faixa que entrou no ar já está pré-carregada no elemento ocioso
    // → troca de papel, sem novo request. É o que elimina a espera de rede no
    // início da faixa (o deck ocioso deixa de ser só metadado).
    if (idle && cuedTrack.current === trackId && idle.src) {
      stemSrcRef.current = false; // pré-carga é sempre do original
      a.pause();
      idle.volume = vol;
      try {
        // já é a faixa NOVA no `np`, então `cueInRef` é o cue dela: a virada
        // entra direto no áudio, sem o vazio da frente.
        idle.currentTime = cueInRef.current / 1000;
      } catch {
        /* ainda sem metadata — o play começa do 0 mesmo */
      }
      // o loadedmetadata do pré-carregado já disparou ANTES do listener mudar de
      // elemento; semeia a duração real aqui p/ o deck não ficar sem barra.
      setRealDur(idle.duration && isFinite(idle.duration) ? idle.duration * 1000 : 0);
      cuedTrack.current = "";
      const nidx: 0 | 1 = liveIdxRef.current === 0 ? 1 : 0;
      liveIdxRef.current = nidx; // antes do setState: fade/toggle já miram o novo
      setLiveIdx(nidx);
      return;
    }

    setRealDur(0);
    // RETOMADA pós-refresh: se esta é a mesma faixa que estava no ar antes do F5,
    // volta pra posição salva. `currentTime` antes do `loadedmetadata` é ignorado
    // (ou lança) — então o valor fica pendente e é aplicado no `loadedmetadata`,
    // no effect de eventos. A URL do áudio serve Range, então o seek não rebaixa
    // o arquivo inteiro.
    const resumeMs = resumeFor(npUri, durMs);
    /* TRIM DE ENTRADA: sem retomada, a faixa começa no `cueIn` (fim do vazio da
       frente) em vez de 00:00. Entra pelo MESMO `pendingSeek` da retomada — o
       `loadedmetadata` já sabe aplicá-lo, e escrever `currentTime` antes disso é
       ignorado (ou lança) pelo browser.
       A retomada VENCE o trim: quem recarregou a página quer voltar onde estava,
       e essa posição já está depois do corte. */
    const startMs = resumeMs > 0 ? resumeMs : cueInRef.current;
    pendingSeek.current = startMs;
    startedAtMs.current = startMs; // encurta o fallback de avanço (não espera a faixa inteira)
    if (resumeMs > 0) {
      // retomada NÃO é troca de faixa: entra a volume pleno, sem fade-in — subir
      // do zero no meio da música soaria como falha de rede.
      fadeInPending.current = false;
      a.volume = gainRef.current;
      setPosMs(resumeMs);
    } else {
      a.volume = vol;
    }
    /* Se esta faixa já foi bufferizada enquanto estava engatilhada, entra pelo
       BLOB — imune a queda de internet e sem novo request.

       ⚠️ O BLOB é sempre do ORIGINAL. Com instrumental pronto e o modo VOZ
       ligado, a fonte tem de ser o arquivo separado, então o buffer é ignorado
       de propósito: preferir o cache aqui tocaria a versão COM voz e o operador
       veria o modo simplesmente não funcionar. */
    const wantStem = vocalCutRef.current && hasStemRef.current;
    const ready = wantStem ? "" : bufferedURL(trackId);
    a.src = ready || trackAudioURL(trackId, wantStem);
    // ANOTA A VERDADE: quem carrega é quem sabe qual fonte entrou no elemento. O
    // efeito de troca lê isto; sem a anotação ele acha que "não sei" é "trocar".
    stemSrcRef.current = wantStem;
    if (!ready && !wantStem) {
      /* Sem buffer em MEMÓRIA (caso do boot: um F5 zera os Blobs), procura em
         DISCO. É o que faz a faixa no ar voltar a tocar depois de recarregar sem
         servidor — o streaming acima falharia, e o cache tem o arquivo inteiro.

         ⚠️ TROCAR O `src` ABORTA UM `play()` EM VOO. Medido no F5: a retomada
         chama `play()`, esta promessa resolve um instante depois e a troca derruba
         aquele play com `AbortError: The play() request was interrupted by a new
         load request`. Como o WebSocket ainda não conectou nesse instante, o
         `catch` classificava pelo enlace e anunciava "Esta faixa não está no cache
         local" — o oposto exato da verdade: ela ESTAVA no cache, e trocar para ele
         é que matou o play.

         O guarda antigo (`el.currentTime > 0`) não pega o caso: no F5 a posição
         ainda é 0 porque o seek só é aplicado no `loadedmetadata`. Por isso ele
         ganhou dois companheiros: `el.paused` (já pedimos pra tocar? então não
         puxe o tapete) e o re-play explícito quando a troca acontece mesmo assim. */
      void bufferTrack(trackId, bufferIds).then((url) => {
        const el = liveAudio();
        if (!url || !el || lastLoaded.current !== trackId) return;
        if (el.currentTime > 0 || !el.paused) return; // já andou, ou já vai tocar
        const at = pendingSeek.current;
        el.src = url;
        pendingSeek.current = at; // a retomada continua valendo com a fonte nova
      });
    }
  }, [isLocal, trackId, npUri, durMs, clearFade, clearTimer, liveAudio, idleAudio]);

  /* PRÉ-CARGA da próxima: `preload="auto"` faz o browser adiantar o arquivo, e o
     `bufferTrack` guarda a faixa INTEIRA como Blob.

     Por que os dois: o `preload` acelera a virada, mas o que o browser guardou é
     um buffer de streaming que ele pode descartar — e, se a internet cair antes da
     virada, a próxima faixa simplesmente não existe do lado de cá. Com o Blob a
     faixa engatilhada fica na memória do browser e toca mesmo com o servidor fora.

     A faixa NO AR segue em streaming de propósito: baixá-la inteira antes de tocar
     atrasaria o início num link lento — e ela já está tocando, ou seja, o browser
     já bufferizou à frente. */
  useEffect(() => {
    const idle = idleAudio();
    if (!idle) return;
    if (!nextTrackId || nextTrackId === trackId) return;
    if (cuedTrack.current === nextTrackId) return;
    cuedTrack.current = nextTrackId;
    idle.pause();
    idle.volume = 0;
    idle.src = trackAudioURL(nextTrackId);
    idle.load();
    // baixa em background; se falhar, o elemento continua no streaming acima.
    void bufferTrack(nextTrackId, [trackId]).then((url) => {
      // só troca se a faixa AINDA é a engatilhada e o elemento não entrou no ar
      // no meio do download (a virada pode acontecer antes de terminar).
      if (url && cuedTrack.current === nextTrackId && idle.paused) {
        const at = idle.currentTime;
        idle.src = url;
        try {
          idle.currentTime = at;
        } catch {
          /* sem metadata ainda — recomeça do 0, que é onde ela estava */
        }
      }
    });
  }, [nextTrackId, trackId, liveIdx, idleAudio]);

  /* BUFFER ANTI-QUEDA: mantém em memória a faixa no ar + as 3 próximas.

     O conjunto é RECALCULADO a cada mudança da fila (`bufferKey`), não acumulado:
     reordenar, remover ou adiantar uma faixa muda quem são "as 3 próximas", e o
     cache tem de acompanhar — senão guardaríamos música que saiu da vista e
     deixaríamos a que entrou de fora, justamente a que vai tocar.

     `keepOnly` primeiro (libera o que não serve mais), download depois. E em
     SÉRIE, não em paralelo: baixar 3 arquivos de uma vez disputaria banda e
     conexões com a faixa que está tocando — o buffer existe pra proteger o ar,
     não pra atrapalhar. */
  useEffect(() => {
    const wanted = bufferIds.filter(Boolean);
    keepOnly([trackId, ...wanted].filter(Boolean));
    if (wanted.length === 0) return;
    let cancelled = false;
    void (async () => {
      for (const id of wanted) {
        if (cancelled) return; // fila mudou no meio: o próximo efeito assume
        await bufferTrack(id, [trackId, ...wanted]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [bufferKey, trackId]);

  /* Autoplay bloqueado (política do browser): re-tenta SOZINHO no primeiro gesto do
     operador em qualquer lugar da página — mesma mecânica que o motor do YouTube já
     usava. Qualquer clique/tecla destrava; não precisa ser no PLAY.

     **Sem aviso na tela** (decisão do operador): a barra "clique para retomar o
     áudio" aparecia a cada refresh e virava ruído — quem está operando quer que
     volte, não que peça permissão. O destravamento acontece em silêncio no
     primeiro toque, que numa operação real vem em segundos.

     Listeners `once` + guarda pra não empilhar um por tentativa. */
  const gestureArmed = useRef(false);
  const armResumeOnGesture = useCallback(() => {
    if (gestureArmed.current || typeof window === "undefined") return;
    gestureArmed.current = true;
    const unlock = () => {
      gestureArmed.current = false;
      const a = liveAudio();
      if (!a) return;
      // NÃO checar `a.paused` aqui: no bloqueio do Chrome o elemento fica
      // `paused=false` (o play foi aceito) e mesmo assim o carregamento é ADIADO —
      // `readyState` fica 0 e nada toca. Chamar play() de novo, agora dentro do
      // gesto, é o que destrava; num elemento que já toca é inofensivo.
      void a
        .play()
        .then(() => {
          setPlaying(true);
          setBlocked(false);
        })
        .catch(() => {});
    };
    const opts = { once: true, capture: true } as const;
    window.addEventListener("pointerdown", unlock, opts);
    window.addEventListener("keydown", unlock, opts);
  }, [liveAudio]);

  // transporte mestre: wantPlay liga/desliga o playout local (espelha o gate do
  // Spotify). wantPlay=true → toca a faixa carregada; false → pausa (fica cued).
  useEffect(() => {
    const a = liveAudio();
    if (!a || !isLocal) return;
    if (wantPlay && !userPaused) {
      if (a.paused)
        void a
          .play()
          .then(() => {
            setPlaying(true);
            // faixa nova arrancou → sobe do 0 ao volume pleno em 3s (fade-in).
            if (fadeInPending.current) {
              fadeInPending.current = false;
              void rampVolume(gainRef.current, fadeMs());
            }
          })
          .catch((err: unknown) => {
            setPlaying(false);
            // Antes este catch era mudo: arquivo que não carrega (404, formato
            // recusado, rede) deixava a faixa parada em 00:00 SEM uma palavra, e o
            // operador não tinha por onde começar.
            /* AbortError NÃO É FALHA — é "um load/play mais novo passou na frente
               deste". Acontece por desenho toda vez que o `src` é trocado com um
               play pendente (o buffer de disco no boot, o handoff do ping-pong).
               Tratar como erro produzia o pior tipo de mensagem: confiante e
               errada — "Esta faixa não está no cache local" logo depois de trocar
               PARA o cache. Quem manda de verdade é o detector de progresso, que
               acende o ▶ se o som realmente não sair. */
            if (err instanceof DOMException && err.name === "AbortError") return;
            const autoplayBlocked =
              err instanceof DOMException && err.name === "NotAllowedError";
            if (autoplayBlocked) {
              /* MEDIDO (0.2.26): não existe contorno pra `<audio>`. O truque do
                 "autoplay mudo é sempre permitido" vale pra `<video>`; num
                 elemento de áudio o Chrome recusa IGUAL — `a.muted = true` +
                 `play()` devolve **NotAllowedError também**. Web Audio não ajuda
                 (o AudioContext nasce `suspended` pelo mesmo motivo).
                 Então: re-tenta no primeiro gesto, em silêncio. Um Cmd+R acontece
                 no chrome do browser e NÃO conta como interação com a página.
                 Onde o Chrome já confia no site (engagement alto — o caso de uso
                 diário em produção, e o de app instalado), ele libera sozinho e
                 este caminho nem roda. */
              // Caso clássico do REFRESH: a posição é retomada, mas o browser não
              // deixa sair som sem um gesto na página nova. Antes isso era `return`
              // seco — o deck ficava parado no ponto certo, mudo, sem dizer nada, e
              // parecia que "o refresh matou o playout". Agora a retomada fica
              // ARMADA no primeiro clique/tecla em qualquer lugar, e o operador é
              // avisado do porquê.
              armResumeOnGesture();
              return;
            }
            // Offline a causa é quase sempre uma só, e dizer isso poupa o
            // operador de caçar problema em arquivo que está íntegro: a faixa
            // simplesmente não estava no cache local quando a rede caiu.
            if (!isServerLinked()) {
              toast.warning("Esta faixa não está no cache local — sem servidor, não dá para tocá-la.");
              return;
            }
            const cod = a.error?.code;
            toast.error(
              // O código 4 do `<audio>` cobre DOIS casos que o elemento não
              // distingue: formato recusado e recurso que não veio (arquivo
              // ausente, 404). Medido ao esconder um MP3 do acervo: dava
              // "formato não suportado" numa faixa que o navegador tocava
              // minutos antes — mensagem confiante e errada. Melhor citar as
              // duas hipóteses do que apontar a errada com firmeza.
              cod === 4
                ? "Não foi possível carregar o áudio: arquivo indisponível ou formato não suportado."
                : cod === 2
                  ? "Falha de rede ao carregar o áudio da faixa."
                  : "Não foi possível carregar o áudio desta faixa.",
            );
          });
    } else if (!a.paused) {
      a.pause();
    }
  }, [
    wantPlay,
    isLocal,
    trackId,
    userPaused,
    rampVolume,
    liveIdx,
    liveAudio,
    armResumeOnGesture,
  ]);

  /* DETECTOR DE "NÃO SAIU SOM" — por PROGRESSO, não por exceção.

     O `catch` do `play()` não basta: medido no Chrome, quando o autoplay é barrado
     depois de um refresh o elemento fica `paused = false` (o pedido foi aceito), a
     promessa **nunca resolve nem rejeita** e o carregamento é ADIADO — `readyState`
     travado em 0, `networkState` em "carregando", zero áudio. Nenhum erro pra ler.

     Então o sinal honesto é a POSIÇÃO NÃO ANDAR enquanto a estação diz que quer
     tocar. Vale também pra buffer travado ou arquivo que não decodifica — em todos,
     o que o operador precisa é o mesmo: o deck mostrar ▶ e um clique resolver. */
  useEffect(() => {
    if (!isLocal || !wantPlay || userPaused) {
      setBlocked(false);
      return;
    }
    let lastTime = -1;
    const id = setInterval(() => {
      const a = liveAudio();
      if (!a) return;
      const now = a.currentTime;
      if (now !== lastTime && lastTime >= 0) {
        setBlocked(false); // andou: está tocando de verdade
        lastTime = now;
        return;
      }
      if (lastTime < 0) {
        lastTime = now;
        return; // 1ª amostra é só referência
      }
      setBlocked(true);
      armResumeOnGesture();
    }, 2500);
    return () => clearInterval(id);
  }, [isLocal, wantPlay, userPaused, trackId, liveIdx, liveAudio, armResumeOnGesture]);

  // eventos do <audio>: posição real, duração real, play/pause e FIM (→ avança).
  // Reassina a cada troca de papel (liveIdx) — os listeners seguem o elemento no ar.
  useEffect(() => {
    const a = liveAudio();
    if (!a) return;
    const onTime = () => {
      const pos = a.currentTime * 1000;
      setPosMs(pos);
      // guarda onde está: é isto que faz o F5 continuar de onde parou. Salva também
      // pausado (pausar e recarregar tem de voltar no mesmo ponto).
      if (npUri) saveResume(npUri, pos, !a.paused);

      /* LOOP na base instrumental. Com o loop armado e um vocal marcado, ao
         alcançar a entrada da voz a faixa volta ao começo do ÁUDIO (o cueIn,
         não o 00:00 — voltar pro vazio da frente daria um buraco de silêncio no
         meio da locução). É o que segura a base enquanto o operador fala. */
      const vocal = vocalRef.current;
      if (loopRef.current && vocal > 0 && pos >= vocal) {
        try {
          a.currentTime = cueInRef.current / 1000;
        } catch {
          /* seek recusado (buffer): tenta de novo no próximo tick */
        }
        return;
      }

      /* FIM EFETIVO (trim de saída): a faixa acaba no `cueOut`, não na duração
         do arquivo. Vai pelo MESMO guard `advancedFor` do evento `ended` e do
         timer de fallback — os três caminhos avançam a fila, e só um pode
         ganhar. Sem o guard, cortar aqui somaria um segundo avanço quando o
         `ended` chegasse. */
      const out = cueOutRef.current;
      if (out > 0 && pos >= out && trackId && advancedFor.current !== trackId) {
        advancedFor.current = trackId;
        post("/api/v1/playback/next");
      }
    };
    const onMeta = () => {
      if (a.duration && isFinite(a.duration)) setRealDur(a.duration * 1000);
      // aplica a retomada agora que a duração é conhecida — antes disso o browser
      // ignora a escrita em `currentTime`.
      const target = pendingSeek.current;
      if (target > 0) {
        pendingSeek.current = 0;
        const dur = a.duration && isFinite(a.duration) ? a.duration * 1000 : 0;
        if (dur <= 0 || target < dur - 3000) {
          try {
            a.currentTime = target / 1000;
          } catch {
            /* alguns formatos recusam seek antes do buffer; segue do início */
          }
        }
      }
    };
    const onPlay = () => {
      setPlaying(true);
      setBlocked(false); // saiu som: o botão volta a ser ⏹ STOP
    };
    /* ⚠️ IGNORA O `pause` DA TROCA DE FONTE (botão SEM-VOZ).

       Na troca, o elemento que SAI é pausado depois da rampa — mas ele ainda é o
       "ao vivo" nesse instante, então o evento dele chegava aqui e marcava a
       estação como parada: o transporte trocava PAUSAR por RETOMAR no meio de uma
       música que seguia tocando (pelo outro elemento). O operador via o app
       anunciar uma pausa que não existiu.

       O guarda é um ref, e não um estado, porque isto acontece dentro de uma
       sequência assíncrona — um estado chegaria tarde demais. */
    const onPause = () => {
      if (swappingRef.current) return;
      setPlaying(false);
    };
    const onEnded = () => {
      if (!trackId || advancedFor.current === trackId) return;
      // Sem servidor o `post` avança no ESPELHO e anota a dívida (0.2.27) — a
      // faixa acabou de verdade, então a fila TEM de andar mesmo offline. Segurar
      // aqui deixava o cabeçalho preso na música que já tinha terminado.
      advancedFor.current = trackId;
      post("/api/v1/playback/next");
    };
    a.addEventListener("timeupdate", onTime);
    a.addEventListener("loadedmetadata", onMeta);
    a.addEventListener("play", onPlay);
    a.addEventListener("pause", onPause);
    a.addEventListener("ended", onEnded);
    return () => {
      a.removeEventListener("timeupdate", onTime);
      a.removeEventListener("loadedmetadata", onMeta);
      a.removeEventListener("play", onPlay);
      a.removeEventListener("pause", onPause);
      a.removeEventListener("ended", onEnded);
    };
  }, [trackId, npUri, post, liveIdx, liveAudio]);

  /* Fallback de avanço pela duração conhecida — se o arquivo não tocar (sem PCM/
     permissão/arquivo ausente), a agenda ainda anda A→B no tempo certo. O guard
     compartilhado com o evento `ended` garante 1 avanço só.

     ⚠️ ESTE TIMER É RELÓGIO DE PAREDE e por isso é o caminho mais fácil pra uma
     queda de internet TROCAR A FAIXA — o pior resultado possível. Se o buffer secar,
     o áudio fica parado enquanto o relógio segue; quando a rede volta, o timer já
     "venceu" e o avanço sairia no exato instante em que o operador voltava a ouvir a
     música. Daí as duas guardas abaixo: sem enlace não avança, e não avança enquanto
     o áudio estiver claramente ATRASADO em relação ao fim (ele parou, não terminou).
     Nesses casos o timer se reagenda em vez de pular. */
  useEffect(() => {
    if (!isLocal || !wantPlay || userPaused || durMs <= 0) return;
    // O fim que vale é o EFETIVO: com trim de saída a faixa acaba no `cueOut`, e
    // um timer armado pela duração cheia só dispararia depois do silêncio que
    // acabou de ser cortado — a fila ficaria parada esperando o vazio passar.
    const endMs = cueOutRef.current > 0 ? cueOutRef.current : durMs;
    // desconta o que já correu: numa retomada a 3:00 de uma faixa de 3:30 o timer
    // cheio esperaria 3:31 e a fila ficaria travada minutos se o áudio não tocasse.
    const remaining = Math.max(0, endMs - startedAtMs.current);
    let id: ReturnType<typeof setTimeout>;

    const tryAdvance = () => {
      if (!trackId || advancedFor.current === trackId) return;
      // LOOP armado segura a faixa de propósito: o relógio de parede não pode
      // avançar por cima do operador que está falando.
      if (loopRef.current) {
        id = setTimeout(tryAdvance, 2000);
        return;
      }
      const a = liveAudio();
      /* ATRASADO ≠ TERMINADO. Este timer é relógio de parede: numa queda o buffer
         pode secar e o áudio ficar para trás enquanto o relógio corre. Avançar aqui
         cortaria a música no meio — o pior resultado possível. Só o `ended` (ou o
         timer com o áudio realmente no fim) manda a fila andar.
         Sem servidor NÃO é motivo pra segurar: o `post` avança no espelho e paga a
         dívida depois. */
      // "fim" aqui também é o EFETIVO: com trim de saída o áudio nunca chega em
      // `a.duration`, então comparar com ela deixaria o `behind` verdadeiro para
      // sempre e a fila só andaria pelo caminho do tick.
      const endSec = cueOutRef.current > 0 ? cueOutRef.current / 1000 : a?.duration ?? 0;
      const behind =
        !!a && a.currentTime > 0 && endSec > 0 && a.currentTime < endSec - 2;
      if (behind) {
        id = setTimeout(tryAdvance, 2000); // tenta de novo quando alcançar o fim
        return;
      }
      advancedFor.current = trackId;
      post("/api/v1/playback/next");
    };

    id = setTimeout(tryAdvance, remaining + 1200);
    return () => clearTimeout(id);
  }, [isLocal, wantPlay, userPaused, trackId, durMs, post, liveAudio]);

  const toggle = useCallback(() => {
    const a = liveAudio();
    if (!a || !isLocal) return;
    if (a.paused) {
      setUserPaused(false); // RETOMAR → re-arma auto-avanço
      clearFade(); // retomada manual = ganho pleno (não fica preso num fade parado)
      a.volume = gainRef.current;
      void a.play().catch(() => {});
    } else {
      setUserPaused(true); // PAUSE → congela avanço até retomar/trocar faixa
      a.pause();
    }
  }, [isLocal, clearFade, liveAudio]);

  // fade-out de 3s antes de trocar a faixa no ar (PRÓXIMA/RETROCEDER) — desce o
  // volume até 0 no lugar do corte seco. Só faz sentido tocando; parado é no-op.
  const fadeOut = useCallback(async (): Promise<void> => {
    const a = liveAudio();
    if (!a || !isLocal || a.paused || !fadeEnabled()) return;
    await rampVolume(0, fadeMs());
  }, [isLocal, rampVolume, liveAudio]);

  const seek = useCallback(
    (frac: number) => {
      const a = liveAudio();
      const d = realDur > 0 ? realDur : durMs;
      if (!a || !isLocal || d <= 0) return;
      a.currentTime = (Math.max(0, Math.min(1, frac)) * d) / 1000;
      advancedFor.current = ""; // seek p/ trás re-arma o auto-avanço
    },
    [isLocal, realDur, durMs, liveAudio],
  );

  const deck: PlayoutDeck = useMemo(() => {
    const dur = realDur > 0 ? realDur : durMs;
    posRef.current = posMs;
    endRef.current = rawCueOut > 0 && rawCueOut < dur ? rawCueOut : dur;
    return {
      isSpotify: false,
      live: isLocal,
      playing,
      posMs,
      // duração REAL do arquivo: é o que a tela mostra e o que a barra mede.
      durMs: dur,
      // fim EFETIVO: menor que a duração quando o trim corta o vazio do fim. É
      // este que o coordenador da mixagem tem de usar — com a duração crua o
      // crossfade começaria DEPOIS do fim já cortado, e a virada viraria corte
      // seco exatamente nas faixas com silêncio no fim.
      endMs: rawCueOut > 0 && rawCueOut < dur ? rawCueOut : dur,
      vocalMs,
      // O botão VOZ só liga com o instrumental JÁ PREPARADO — é o arquivo que
      // vai tocar no lugar do original. Sem preparo não há o que tocar, e o
      // botão diz isso em vez de aplicar um efeito que quase não remove voz.
      instrumentalReady: hasStem,
      // ⚠️ NÃO combinar com `!playing`: o Chrome dispara o evento `play` assim que
      // o play() é aceito, mesmo quando adia o carregamento e nada soa — `playing`
      // fica true e anularia o bloqueio justamente no caso que ele existe pra pegar.
      // Quem manda é o detector de progresso.
      blocked: isLocal && blocked,
    };
  }, [isLocal, playing, posMs, realDur, durMs, blocked, rawCueOut, vocalMs, hasStem]);

  return { deck, toggle, seek, fadeOut, startMix };
}
