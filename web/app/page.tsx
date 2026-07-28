"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "@/lib/toast";
import { confirmDialog } from "@/lib/confirm";
import { setPlayingNow } from "@/lib/onAir";
import { humanizeThrown } from "@/lib/errors";
import {
  importSpotifyURI,
  importYouTubeURI,
  youtubeImportLinks,
  spotifyConnectPause,
  spotifyDisconnect,
  spotifyRevalidate,
  youtubeDisconnect,
} from "@/lib/api";
import { useAuth } from "@/hooks/useAuth";
import { SecondScreen } from "@/components/SecondScreen";
import { useProfile } from "@/hooks/useProfile";
import { useApplyProfile } from "@/hooks/useApplyProfile";
import { useClock } from "@/hooks/useClock";
import { useNowPlaying } from "@/hooks/useNowPlaying";
import { usePlayHistory } from "@/hooks/usePlayHistory";
import { useSpotifyPlayout, type PlayoutDeck } from "@/hooks/useSpotifyPlayout";
import { useYouTubePlayout } from "@/hooks/useYouTubePlayout";
import { useYouTubeOverlay } from "@/hooks/useYouTubeOverlay";
import { useCrossEngineMix } from "@/hooks/useCrossEngineMix";
import { useLocalPlayout } from "@/hooks/useLocalPlayout";
import { useMix } from "@/hooks/useMix";
import { useAutopilot } from "@/hooks/useAutopilot";
import { useResizablePanel } from "@/hooks/useResizablePanel";
import { ServerLoginRequired } from "@/components/ServerLoginRequired";
import { OperatorTitlebar } from "@/components/OperatorTitlebar";
import { UserBadge } from "@/components/UserBadge";
import { LayoutEditor } from "@/components/LayoutEditor";
import { PlayerScreen } from "@/components/PlayerScreen";
import { RightPanel, type PanelMode } from "@/components/RightPanel";
import { PanelDivider } from "@/components/PanelDivider";
import { useConnectivity } from "@/hooks/useConnectivity";
import { ConfirmHost } from "@/components/ConfirmHost";

/* orquestra a UI do operador — gate de login (Story 7.3) na frente; depois Player
   fixo à esquerda + painel direito multi-modo (Operacional | Internet),
   redimensionável e colapsável (☰). */
export default function Operator() {
  // Second-monitor wallboard (Story 9.2): the operator opens ?monitor=2 in a new
  // window. That URL renders the read-only SecondScreen instead of the console —
  // two windows can't share a DOM, so this is a dedicated view over the same live
  // WebSocket, not a mirror. window.location is read after mount to stay safe for
  // Next static export (no `window` during prerender).
  const [monitor, setMonitor] = useState<number>(1);
  useEffect(() => {
    const m = new URLSearchParams(window.location.search).get("monitor");
    if (m === "2") setMonitor(2);
  }, []);
  if (monitor === 2) return <SecondScreen />;

  return <Console />;
}

function Console() {
  const auth = useAuth();
  const prof = useProfile(!!auth.user);
  useApplyProfile(prof.profile);
  const now = useClock();
  const { snap, connected, busy, post, enqueue, dequeue, move } = useNowPlaying();
  // rede: avisa quando cai (toast fixo) e recarrega quando volta, pro áudio retomar.
  useConnectivity(connected);
  // histórico de tocadas da sessão (rastro visual acima do no-ar)
  const { items: history, clear: clearHistory } = usePlayHistory(snap);
  // espelho do histórico p/ o RETROCEDER pegar a última tocada sem stale closure.
  const historyRef = useRef(history);
  useEffect(() => {
    historyRef.current = history;
  }, [history]);
  // playout real no browser — DOIS motores espelhando a mesma agenda:
  //  • Spotify (SDK, DRM/hard_cut) quando o now-playing é uma faixa Spotify;
  //  • Local (<audio>, PCM) quando é uma faixa local.
  // Cada um toca a faixa now-playing, alimenta a posição do deck ativo e AVANÇA a
  // fila (POST /playback/next) quando a faixa termina. O deck "ativo" é o motor
  // cujo now-playing casa; o outro fica inerte. Antes só o Spotify avançava —
  // faixa local terminava e a fila travava (Deck A parava, Deck B não entrava).
  // PLAY mestre da estação (transporte), provider-agnóstico. wantPlay é o gate
  // ÚNICO passado aos DOIS motores (Spotify + local) — mesmo recurso p/ qualquer
  // provider (hexagonal). true = no ar (arranca a cabeça da fila e avança sozinho);
  // false = "nada no ar"/parado (faixa engatilhada a 00:00 esperando PLAY).
  // Persistido: se estava no ar antes do refresh, volta no ar. Default false.
  const [wantPlay, setWantPlay] = useState(false);
  useEffect(() => {
    try {
      setWantPlay(window.localStorage.getItem("autodj.wantplay.v1") === "1");
    } catch {
      /* ignora */
    }
  }, []);
  const setWant = useCallback((v: boolean) => {
    setWantPlay(v);
    try {
      window.localStorage.setItem("autodj.wantplay.v1", v ? "1" : "0");
    } catch {
      /* ignora */
    }
  }, []);

  // piloto automático (AUTO). Ler o estado aqui p/: (a) AUTO ligado ⇒ no ar
  // automático; (b) drop de faixa só arranca se AUTO On (senão espera o operador
  // dar PLAY). Ref espelha o enabled p/ os wrappers de enqueue sem stale closure.
  const autopilot = useAutopilot();
  const autoOn = autopilot.status.enabled;
  const autoOnRef = useRef(autoOn);
  useEffect(() => {
    autoOnRef.current = autoOn;
    // AUTO ligado ⇒ estação no ar (arranca a fila). AUTO desligado NÃO força
    // parada (o operador pode estar tocando manual) — só o esvaziar da fila para.
    if (autoOn) setWant(true);
  }, [autoOn, setWant]);

  // fila esvaziou (nada no ar) ⇒ para o transporte = "nada no ar para", igual ao
  // deck ocioso quando não toca nada. Re-arma o gate p/ o próximo PLAY/drop.
  // Só quando AUTO Off: com AUTO On o esvaziar é transitório (o piloto reabastece
  // logo), então NÃO paramos — senão a estação morreria sozinha entre refills.
  const hasHead = !!snap?.now_playing?.track_id;
  // ⚠️ SÓ DEPOIS DO 1º SNAPSHOT (0.2.26). No boot `snap` é null, então `hasHead`
  // era false e esta regra disparava `setWant(false)` ANTES de o WS dizer o que há
  // na fila — todo refresh derrubava o transporte que estava ligado e a estação
  // "parava sozinha" ao recarregar (com AUTO Off). A retomada de posição existia e
  // funcionava; o que faltava era o transporte continuar ligado pra usá-la.
  const snapshotSeen = !!snap;
  useEffect(() => {
    if (!snapshotSeen) return;
    if (!hasHead && !autoOn) setWant(false);
  }, [snapshotSeen, hasHead, autoOn, setWant]);

  // "no ar de verdade" = tem faixa na cabeça E transporte ligado. Guarda o
  // "Tocar agora" (menu ou drop no deck): trocar a faixa no ar interrompe o que
  // está tocando, então pede confirmação antes. Ref espelha p/ os wrappers de
  // enqueue não pegarem closure velha.
  const onAirRef = useRef(false);
  useEffect(() => {
    onAirRef.current = hasHead && wantPlay;
  }, [hasHead, wantPlay]);
  // pergunta antes de cortar o que toca. true = segue com o "tocar agora";
  // false = operador cancelou. No ar parado ⇒ nunca pergunta (nada a cortar).
  const confirmPlayNow = useCallback(async (): Promise<boolean> => {
    if (!onAirRef.current) return true;
    return confirmDialog({
      title: "Trocar a faixa no ar?",
      body: "Vai interromper na hora o que está tocando agora e começar esta faixa.",
      okText: "Tocar agora",
      cancelText: "Cancelar",
      danger: true,
    });
  }, []);

  // handoff: Spotify começou a tocar em outro device da conta → desliga o PLAY
  // mestre aqui (não competir pelo controle) e avisa. useSpotifyPlayout detecta
  // via SDK e chama isto.
  const onSpotifyTakeover = useCallback(() => {
    setWant(false);
    toast.info("Spotify assumido por outro dispositivo — pausado aqui.");
  }, [setWant]);
  // Motor do Spotify TRAVADO: device conectado, faixa carregada nele, comandos
  // aceitos sem erro e áudio nenhum. Antes isso era silencioso — e o fallback por
  // duração ia pulando faixa até esvaziar a fila ("mix mudo e depois some tudo").
  // Agora: PARA a estação (não escoa a fila), revalida o token pra descobrir o
  // motivo REAL e diz ao operador o que fazer.
  const onSpotifyStalled = useCallback(() => {
    setWant(false);
    void spotifyRevalidate().then((st) => {
      if (!st) {
        toast.error(
          "Spotify recusou a sessão — reconecte a conta no painel (Spotify → Desconectar → Conectar).",
        );
        return;
      }
      if (!st.premium) {
        toast.error("Spotify sem Premium ativo — o player do navegador exige Premium.");
        return;
      }
      toast.error("Spotify não iniciou a reprodução. Estação pausada para não perder a fila.");
    });
  }, [setWant]);
  const {
    deck: spotDeck,
    toggle: spotToggle,
    seek: spotSeek,
    fadeOut: spotFadeOut,
    startMix: spotStartMix,
  } = useSpotifyPlayout(snap, post, wantPlay, onSpotifyTakeover, onSpotifyStalled);
  // host PERSISTENTE do IFrame do YouTube. Único <div> que NUNCA desmonta E NUNCA
  // se move na árvore (fica estacionado oculto aqui). Reparentar um <iframe>
  // RECARREGA o content document (spec HTML) → matava o playback a cada troca A/B
  // (causa-raiz v0.2.4). Em vez de mover, o hook useYouTubeOverlay sobrepõe o host
  // via position:fixed casando o retângulo do slot ([data-yt-slot]) do deck ao
  // vivo. CSS não recarrega o iframe → o player sobrevive e toca. É a ÚNICA
  // divergência estrutural do espelho Spotify (o IFrame exige elemento DOM visível).
  // SÃO DOIS (v0.2.11): um toca a faixa no ar, o outro fica engatilhado com a
  // próxima da fila já bufferizada (muda) — a virada vira troca de papel, sem o
  // buraco de 1-3s de buffering do loadVideoById. Qual host é qual papel muda a
  // cada faixa; `roles` (abaixo) diz o papel corrente de cada um.
  const ytHostRef = useRef<HTMLDivElement>(null);
  const ytHostBRef = useRef<HTMLDivElement>(null);
  const {
    deck: youtDeck,
    toggle: youtToggle,
    seek: youtSeek,
    fadeOut: youtFadeOut,
    startMix: youtStartMix,
    roles: ytRoles,
  } = useYouTubePlayout(ytHostRef, ytHostBRef, snap, post, wantPlay);
  // Áudio das faixas locais (⚙ Configurações): trim de silêncio por tipo de
  // mídia, ganho constante entre faixas e detecção de vocal. Mesma configuração
  // (e mesma rota) do modal de Mixagem — só que o ⚙ está sempre alcançável, e o
  // modal é gated no modo Rádio.
  const { mix: mixCfg } = useMix();
  /* LOOP amarrado ao FALAR: armar a locução arma o loop na base instrumental, e
     encerrar solta. O botão LOOP continua clicável sozinho porque **FALAR só
     existe no modo Rádio** (`showTalk={isRadio}`) e o padrão do app é Tocador —
     sem o botão próprio, o recurso ficaria inalcançável no modo padrão. */
  const [loopArmed, setLoopArmed] = useState(false);
  /* MODO VOZ (🚫🎤 no transporte): faixa local sai sem o vocal enquanto ligado.
     Irmão do `loopArmed` e, como ele, **sem persistência** — F5 volta desligado.
     É de propósito: o modo é opt-in e nasce de um clique, e é justamente esse
     clique que destrava o `AudioContext` que o corte usa. Guardar o estado faria
     o app tentar rotear áudio por Web Audio no CARREGAMENTO da página, que é o
     cenário de autoplay barrado que a 0.2.28 recusou. */
  const [vocalCut, setVocalCut] = useState(false);
  const {
    deck: localDeck,
    toggle: localToggle,
    seek: localSeek,
    fadeOut: localFadeOut,
    startMix: localStartMix,
  } = useLocalPlayout(snap, post, wantPlay, mixCfg, loopArmed, vocalCut);
  // seletor de motor (3 branches, mesmo slot 60×60 — só chaveia): Spotify (SDK) >
  // YouTube (IFrame) > local (<audio> catch-all). Mutuamente exclusivos: cada hook
  // só fica `live` quando o now-playing casa com sua fonte. O deck YouTube é
  // normalizado ao shape PlayoutDeck (downstream lê só posMs/durMs/live/playing;
  // `isSpotify` nunca é consultado fora deste seletor).
  const playDeck: PlayoutDeck = spotDeck.isSpotify
    ? spotDeck
    : youtDeck.isYouTube
      ? {
          isSpotify: false,
          live: youtDeck.live,
          blocked: youtDeck.blocked,
          playing: youtDeck.playing,
          posMs: youtDeck.posMs,
          durMs: youtDeck.durMs,
        }
      : localDeck;
  /* publica "está tocando agora?" pros três motores. Quem lê é a checagem de versão
     (v0.2.41): versão nova NÃO recarrega a página por cima de música tocando.
     `blocked` entra na conta porque nele a estação QUER tocar e não sai som — é
     justamente a hora em que recarregar não custa nada. */
  useEffect(() => {
    setPlayingNow(playDeck.live && playDeck.playing && !playDeck.blocked);
  }, [playDeck.live, playDeck.playing, playDeck.blocked]);
  // sobrepõe o host do IFrame no slot do deck ao vivo (position:fixed, sem
  // reparentar → sem recarregar o iframe). Ativo SEMPRE que a faixa no ar é YouTube
  // — o overlay NUNCA é escondido: esconder parqueava o iframe (deck PRETO) e um
  // vídeo lento p/ chegar a PLAYING na troca Spotify→YouTube ficava preso sem tocar.
  // Vídeo permanentemente morto (embed:) é PULADO pela fila (effect 3a), não escondido.
  // O host engatilhado sobrepõe o slot do deck OCIOSO ([data-yt-cue-slot]) — a
  // próxima faixa já aparece (e já está bufferizada) antes de entrar no ar.
  useYouTubeOverlay(ytHostRef, ytRoles[0]);
  useYouTubeOverlay(ytHostBRef, ytRoles[1]);
  // MIXAGEM na virada: no fim de cada faixa começa a próxima subindo de 0 enquanto
  // a atual desce — os dois soam juntos durante a janela do ⚙. Vale p/ motores
  // diferentes (local↔Spotify, local↔YouTube, Spotify↔YouTube) E p/ local↔local,
  // que usa os dois <audio> do ping-pong (v0.2.26). A fila do servidor NÃO é
  // adiantada; ela avança no fim natural, como sempre.
  // `mix` alimenta os decks: durante a janela o servidor ainda aponta a faixa que
  // sai como now-playing, então é este estado que faz a entrante aparecer TOCANDO
  // (tempo correndo + selo MIX nos dois decks) em vez de parada em CUE.
  const mix = useCrossEngineMix({
    snap,
    wantPlay,
    playing: playDeck.playing,
    posMs: playDeck.posMs,
    // FIM EFETIVO, não duração: com trim de saída a faixa acaba antes do fim do
    // arquivo, e com `durMs` cru o crossfade começaria DEPOIS desse fim — a
    // virada viraria corte seco justo nas faixas com silêncio no fim. Streaming
    // não tem trim, então `endMs` vem ausente e cai na duração, como sempre.
    durMs: playDeck.endMs ?? playDeck.durMs,
    spotify: { startMix: spotStartMix, fadeOut: spotFadeOut },
    youtube: { startMix: youtStartMix, fadeOut: youtFadeOut },
    local: { startMix: localStartMix, fadeOut: localFadeOut },
  });
  const playToggle = spotDeck.isSpotify ? spotToggle : youtDeck.isYouTube ? youtToggle : localToggle;
  const playSeek = spotDeck.isSpotify ? spotSeek : youtDeck.isYouTube ? youtSeek : localSeek;
  // fade-out do deck ATIVO antes de trocar a faixa no ar (PRÓXIMA/RETROCEDER).
  // Ref-mirror p/ os callbacks não re-criarem a cada mudança de deck/estado.
  const playFadeOut = spotDeck.isSpotify
    ? spotFadeOut
    : youtDeck.isYouTube
      ? youtFadeOut
      : localFadeOut;
  const fadeOutRef = useRef<() => Promise<void>>(() => Promise.resolve());
  useEffect(() => {
    fadeOutRef.current = playFadeOut;
  }, [playFadeOut]);
  // PLAY mestre (Transport): operador dá play → estação no ar.
  // espelhos p/ o ▶ mestre enxergar o estado do motor sem recriar o callback.
  const playDeckRef = useRef(playDeck);
  playDeckRef.current = playDeck;
  const playToggleRef = useRef(playToggle);
  playToggleRef.current = playToggle;
  const onPlayMaster = useCallback(() => {
    setWant(true);
    /* Autoplay bloqueado: `wantPlay` já é true (a estação QUER tocar), então
       `setWant(true)` não muda nada e o efeito de play não re-roda — o ▶ pareceria
       morto. Este clique, porém, É o gesto que o browser exigia: damos o play na
       mão. Só quando o motor está PARADO — o YouTube bloqueado toca MUDO, e ali o
       próprio listener de gesto desmuta; chamar o toggle o PAUSARIA. */
    const d = playDeckRef.current;
    if (d?.blocked && !d.playing) playToggleRef.current?.();
  }, [setWant]);
  // STOP mestre (Transport): tira a estação do ar. Além de zerar o wantPlay
  // (que já pausa os DOIS motores — local <audio>.pause / Spotify sdkStop), DESLIGA
  // o piloto automático: senão o autopilot (ON por padrão) re-armaria o wantPlay no
  // próximo tick/enqueue e o operador "não conseguiria parar". STOP = controle
  // manual autoritativo; religar é PLAY (ou o botão AUTO no titlebar).
  //
  // sdkStop() do Spotify SÓ silencia o próprio device do SDK ("AutoDJ Playout").
  // Se o playback migrou pra OUTRO aparelho (celular/desktop), o STOP não calava —
  // "dei stop e a música continuou no outro player". Por isso também mandamos o
  // pause pelo Connect Web API, que pausa o device ATIVO da conta seja qual for.
  // Best-effort: sem conta/sem device ativo/sem Premium → no-op, o resto do STOP
  // (wantPlay=false + autopilot off) já vale.
  const onStopMaster = useCallback(() => {
    setWant(false);
    // AUTO precisa cair junto (senão o piloto religa a estação no ato), mas isto é
    // ação de TRANSPORTE — não é você dizendo que não quer mais AUTO. Por isso
    // `toggleForTransport`: não grava preferência, e a próxima sessão volta com
    // AUTO ligado (0.2.27).
    if (autoOnRef.current) void autopilot.toggleForTransport(false).catch(() => {});
    void spotifyConnectPause();
  }, [setWant, autopilot]);
  // wrapper único: toasta erro do enqueue (drag ou menu) e repropaga, pra quem
  // chamou pular o toast de sucesso. Centraliza o feedback num só lugar.
  const enqueueSafe = useCallback(
    async (trackId: string, pos: number, playNow: boolean) => {
      // "tocar agora" com estação no ar ⇒ confirma antes de cortar a faixa atual.
      if (playNow && !(await confirmPlayNow())) return;
      try {
        await enqueue(trackId, pos, playNow);
        // arranca a estação SÓ se AUTO On (drop) ou se pediram tocar agora. AUTO
        // Off ⇒ enfileira e AGUARDA o operador dar PLAY (não toca sozinho).
        if (playNow || autoOnRef.current) setWant(true);
      } catch (e) {
        toast.error(humanizeThrown(e, "não foi possível adicionar à fila"));
        throw e;
      }
    },
    [enqueue, setWant, confirmPlayNow],
  );
  // enfileira do Spotify: importa a URI (faixa/playlist) para a biblioteca e
  // enfileira a(s) faixa(s) criada(s). É o "cidadão de 1ª classe" — toca
  // out-of-band pelo SDK (hard_cut). Usado pelo menu de resultado e pelo drag
  // do resultado Spotify sobre o playout. Preserva ordem: pos<0 anexa cada uma
  // ao fim; pos>=0 insere o bloco em sequência; playNow só na primeira.
  const enqueueSpotifySafe = useCallback(
    async (uri: string, pos: number, playNow: boolean, origin = "") => {
      // idem local: confirma antes de trocar a faixa no ar (antes de importar).
      if (playNow && !(await confirmPlayNow())) return;
      try {
        const tracks = await importSpotifyURI(uri);
        if (tracks.length === 0) throw new Error("importação do Spotify não retornou faixa");
        for (let i = 0; i < tracks.length; i++) {
          await enqueue(tracks[i].id, pos < 0 ? -1 : pos + i, playNow && i === 0, origin);
        }
        // idem local: só entra no ar com AUTO On ou "tocar agora"; senão espera PLAY.
        if (playNow || autoOnRef.current) setWant(true);
        toast.success(
          tracks.length === 1
            ? playNow
              ? `Tocando "${tracks[0].title}"`
              : `"${tracks[0].title}" na fila`
            : origin
              ? `${tracks.length} faixas de "${origin}" na fila`
              : `${tracks.length} faixas do Spotify na fila`,
        );
      } catch (e) {
        toast.error(humanizeThrown(e, "não foi possível adicionar do Spotify"));
        throw e;
      }
    },
    [enqueue, setWant, confirmPlayNow],
  );
  // enfileira do YouTube: espelho fiel do enqueueSpotifySafe. Importa a URI
  // (vídeo/playlist) para a biblioteca e enfileira a(s) faixa(s). Cidadão de 1ª
  // classe — toca out-of-band pelo IFrame (hard_cut, non-mixable). Usado pelo menu
  // de resultado e pelo drag do resultado YouTube sobre o playout. pos<0 anexa cada
  // uma ao fim; pos>=0 insere o bloco em sequência; playNow só na primeira.
  const enqueueYouTubeSafe = useCallback(
    async (uri: string, pos: number, playNow: boolean, origin = "") => {
      if (playNow && !(await confirmPlayNow())) return;
      try {
        const tracks = await importYouTubeURI(uri);
        if (tracks.length === 0) throw new Error("importação do YouTube não retornou faixa");
        for (let i = 0; i < tracks.length; i++) {
          await enqueue(tracks[i].id, pos < 0 ? -1 : pos + i, playNow && i === 0, origin);
        }
        if (playNow || autoOnRef.current) setWant(true);
        toast.success(
          tracks.length === 1
            ? playNow
              ? `Tocando "${tracks[0].title}"`
              : `"${tracks[0].title}" na fila`
            : origin
              ? `${tracks.length} faixas de "${origin}" na fila`
              : `${tracks.length} faixas do YouTube na fila`,
        );
      } catch (e) {
        toast.error(humanizeThrown(e, "não foi possível adicionar do YouTube"));
        throw e;
      }
    },
    [enqueue, setWant, confirmPlayNow],
  );
  // enfileira um LOTE colado de links do YouTube (sub-aba "Colar links", 0.2.17) —
  // o caminho SEM login: uma chamada só resolve todos os ids (`videos.list`, 1
  // unidade de cota) e devolve as faixas já importadas. Anexa ao FIM da fila
  // (pos=-1) preservando a ordem colada; `playNow` só promove a primeira. Devolve
  // quantas entraram — pode ser MENOS que o colado (vídeo não-incorporável é
  // descartado pelo catálogo), que é o que a sub-aba reporta ao operador.
  const enqueueYouTubeLinksSafe = useCallback(
    async (text: string, playNow: boolean): Promise<number> => {
      if (playNow && !(await confirmPlayNow())) return 0;
      try {
        const tracks = await youtubeImportLinks(text);
        if (tracks.length === 0) return 0;
        for (let i = 0; i < tracks.length; i++) {
          await enqueue(tracks[i].id, playNow && i === 0 ? 0 : -1, playNow && i === 0, "");
        }
        if (playNow || autoOnRef.current) setWant(true);
        toast.success(
          tracks.length === 1
            ? playNow
              ? `Tocando "${tracks[0].title}"`
              : `"${tracks[0].title}" na fila`
            : `${tracks.length} faixas do YouTube na fila`,
        );
        return tracks.length;
      } catch (e) {
        toast.error(humanizeThrown(e, "não foi possível importar os links"));
        throw e;
      }
    },
    [enqueue, setWant, confirmPlayNow],
  );
  // wrapper de remoção: toasta erro/sucesso do dequeue (menu botão-direito na
  // fila). Centraliza feedback num só lugar, como o enqueueSafe.
  const dequeueSafe = useCallback(
    async (pos: number, label = "") => {
      try {
        await dequeue(pos);
        toast.success(label ? `"${label}" removida da fila` : "Removida da fila");
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "não foi possível remover da fila");
      }
    },
    [dequeue],
  );
  // wrapper de reordenar: move a ordem de execução (↑/↓ na fila) e só toasta em
  // erro — o reflow da lista já é o feedback de sucesso. Backend: /playback/move.
  const moveSafe = useCallback(
    async (from: number, to: number) => {
      try {
        await move(from, to);
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "não foi possível reordenar a fila");
      }
    },
    [move],
  );
  // adiantar item JÁ na fila arrastando-o sobre um deck. `fromVisible` = índice na
  // cauda visível (slice(2)) → mirror = +2. cut = soltou no deck AO VIVO:
  //  • cut  → move p/ próxima (mirror 1) e /playback/next (corta o atual, promove).
  //           Confirma antes (interrompe o que toca).
  //  • !cut → só move p/ próxima (mirror 1): vira a próxima e espera, sem cortar.
  const promoteSafe = useCallback(
    async (fromVisible: number, cut: boolean) => {
      const mirror = fromVisible + 2;
      if (cut && !(await confirmPlayNow())) return;
      try {
        await move(mirror, 1);
        if (cut) {
          await post("/api/v1/playback/next");
          setWant(true);
        }
      } catch (e) {
        toast.error(e instanceof Error ? e.message : "não foi possível adiantar a faixa");
      }
    },
    [move, post, setWant, confirmPlayNow],
  );
  // ▶ do deck OCIOSO (cued): toca já a próxima que está carregada nele. É a mesma
  // faixa que a UI mostra engatilhada (mirror[1] = engine items[0], em lockstep no
  // backend) — /playback/next promove ela ao ar, não a "próxima-da-fila". Estação
  // no ar ⇒ confirma antes (interrompe o que toca).
  const playCuedSafe = useCallback(async () => {
    if (!(await confirmPlayNow())) return;
    try {
      await post("/api/v1/playback/next");
      setWant(true);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "não foi possível tocar a próxima");
    }
  }, [post, setWant, confirmPlayNow]);
  // RETROCEDER (transport): religa a MÚSICA ANTERIOR (última das Tocadas) ao ar.
  // Enfileira a faixa anterior como cued (pos 1) e promove com /playback/next —
  // mesmo caminho do playCued: a faixa no ar vira "tocada" e a anterior volta a
  // tocar. Como a mesma faixa reentra no histórico, a aba Tocadas passa a mostrar
  // o badge "🔁 N×" (repetida). Estação no ar ⇒ confirma antes (corta o que toca).
  // Enqueue por track_id (a faixa já está na biblioteca — Spotify foi importado ao
  // tocar); se sumiu (Limpar), o enqueue falha e toasta.
  const onPrev = useCallback(async () => {
    const h = historyRef.current;
    const prevItem = h[h.length - 1];
    if (!prevItem) {
      toast.info("Sem faixa anterior para retroceder");
      return;
    }
    // RETROCEDER é ação direta — sem confirmação (pedido do usuário): volta pra
    // anterior na hora. Fade-out de 3s do que toca antes de trocar (sem corte
    // seco); a anterior entra com fade-in de 3s (automático no load do deck).
    try {
      await fadeOutRef.current();
      await enqueue(prevItem.track_id, 1, false, prevItem.origin);
      await post("/api/v1/playback/next");
      setWant(true);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "não foi possível retroceder");
    }
  }, [enqueue, post, setWant]);
  // PRÓXIMA (transport): pula p/ a próxima da fila. Fade-out de 3s do que toca
  // antes do avanço (sem corte seco); a próxima entra com fade-in de 3s (load do
  // deck). STOP continua corte seco de propósito (referência do usuário).
  const onNext = useCallback(async () => {
    try {
      await fadeOutRef.current();
      await post("/api/v1/playback/next");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "não foi possível avançar");
    }
  }, [post]);
  // "Limpar" do header: começa do zero de verdade — zera a playout (fila + faixa
  // no ar), APAGA a biblioteca (Mídias) e DESCONECTA as integrações (Spotify),
  // além do histórico local. Backend: POST /playback/clear + POST /library/clear +
  // POST /spotify/disconnect (idempotente). Recarrega a página no fim porque as
  // instâncias de useLibrary/useSpotify vivem em componentes separados (sem estado
  // compartilhado) — o reload garante que tudo volte vazio de uma vez.
  const clearAll = useCallback(async () => {
    const ok = await confirmDialog({
      title: "Apagar tudo e começar do zero?",
      body: "Esta ação apaga TUDO desta estação e NÃO dá pra desfazer. Vai remover:",
      bullets: [
        "A fila inteira e a faixa que está no ar (o player para).",
        "Toda a biblioteca de Mídias (as faixas importadas).",
        "As integrações com os providers conectados — Spotify e YouTube são DESCONECTADOS (perde o login; reconectar depois pela aba de cada um).",
        "O histórico de Tocadas e as estatísticas de mais tocadas.",
      ],
      okText: "Apagar tudo",
      cancelText: "Cancelar",
      danger: true,
    });
    if (!ok) return;
    try {
      await post("/api/v1/playback/clear");
      await post("/api/v1/library/clear");
      await spotifyDisconnect().catch(() => {}); // idempotente; 404 sem client id → ignora
      await youtubeDisconnect().catch(() => {}); // idem; 404 sem client id/secret → ignora
      clearHistory(); // zera as "tocadas" (histórico local) antes do reload
      toast.success("Tudo limpo");
      window.location.reload(); // instâncias de useLibrary/useSpotify vivem soltas → recarrega do zero
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "não foi possível limpar");
    }
  }, [post, clearHistory]);
  const [mode, setMode] = useState<PanelMode>("operacional");
  const [layoutOpen, setLayoutOpen] = useState(false);
  const web = useResizablePanel({
    min: 320,
    max: 1100,
    initial: 620,
    collapseAt: 220,
    /* MEDIDO, não estimado: a grade da fila (`section.grid`/`li.row`) exige
       536px e o cabeçalho 533px — abaixo disso o player perde colunas em
       silêncio, porque `main.op` recorta em vez de rolar. 560 é esse número com
       uma folga pequena. */
    leftMin: 560,
    storageKey: "autodj.rightpanel.v2",
  });

  const cols = web.collapsed
    ? "minmax(0, 1fr)"
    : `minmax(0, 1fr) 6px ${web.width}px`;

  // Auth gate: hold render until the session bootstrap finishes (no login flash
  // on reload), then require a logged-in operator before exposing the console.
  if (!auth.ready) return <div className="app-boot" />;
  if (!auth.user) return <ServerLoginRequired />;

  return (
    <div className="app" style={{ gridTemplateColumns: cols }}>
      {/* ============ LEFT: Player (fixo) ============ */}
      <main className="op">
        <OperatorTitlebar
          onMenu={() => (web.collapsed ? web.expand() : web.collapse())}
          menuActive={!web.collapsed}
          badge={
            <UserBadge
              user={auth.user}
              onLayout={() => setLayoutOpen(true)}
              onClear={clearAll}
            />
          }
        />
        <PlayerScreen
          now={now}
          snap={snap}
          history={history}
          connected={connected}
          busy={busy}
          post={post}
          spotDeck={playDeck}
          mix={mix}
          onSpotToggle={playToggle}
          onSpotSeek={playSeek}
          wantPlay={wantPlay}
          onPlayMaster={onPlayMaster}
          onStopMaster={onStopMaster}
          onEnqueue={enqueueSafe}
          onEnqueueSpotify={enqueueSpotifySafe}
          onEnqueueYouTube={enqueueYouTubeSafe}
          onDequeue={dequeueSafe}
          onMove={moveSafe}
          onPromote={promoteSafe}
          onPlayCued={playCuedSafe}
          onPrev={onPrev}
          onNext={onNext}
          loopArmed={loopArmed}
          onLoopArmed={setLoopArmed}
          vocalCut={vocalCut}
          onVocalCut={setVocalCut}
          mixCfg={mixCfg}
        />
      </main>

      {/* ============ RIGHT: Operacional | Internet (resizable / collapsible) ============ */}
      {!web.collapsed && (
        <>
          <PanelDivider
            onStart={web.startDrag}
            onMove={web.onMove}
            onEnd={web.endDrag}
            onCollapse={web.collapse}
            active={web.dragging}
          />
          <RightPanel
            mode={mode}
            onMode={setMode}
            onClose={web.collapse}
            onEnqueue={enqueueSafe}
            onEnqueueSpotify={enqueueSpotifySafe}
            onEnqueueYouTube={enqueueYouTubeSafe}
            onEnqueueYouTubeLinks={enqueueYouTubeLinksSafe}
          />
        </>
      )}

      {/* ============ Hosts PERSISTENTES do IFrame do YouTube ============
          Estacionamento oculto dos DOIS <div> de YT player. Nenhum desmonta e
          nenhum se move na árvore — reparentar um <iframe> o recarrega (spec HTML)
          e mata o playback. O hook useYouTubeOverlay sobrepõe cada host via
          position:fixed sobre o slot do deck correspondente ao seu papel:
          [data-yt-slot] p/ o que está no ar, [data-yt-cue-slot] p/ o engatilhado
          (pré-carga da próxima). CSS não recarrega o iframe. É a única
          divergência estrutural do espelho Spotify (o IFrame exige elemento DOM
          visível). Os papéis alternam entre os dois a cada faixa. */}
      <div
        aria-hidden
        style={{ position: "absolute", width: 0, height: 0, overflow: "hidden", left: -9999, top: -9999 }}
      >
        <div ref={ytHostRef} style={{ width: "100%", height: "100%" }} />
        <div ref={ytHostBRef} style={{ width: "100%", height: "100%" }} />
      </div>

      {/* ============ Layout do player por operador (Story 7.4) ============ */}
      {layoutOpen && prof.profile && (
        <LayoutEditor
          profile={prof.profile}
          busy={prof.busy}
          error={prof.error}
          onSave={prof.save}
          onClose={() => setLayoutOpen(false)}
        />
      )}

      {/* Confirm dialogs — montado uma vez. O ToastHost NÃO mora aqui: ele é
          ancorado acima do transporte, dentro do PlayerScreen (rodapé da coluna
          esquerda), que é a posição pedida pro feedback de operação. */}
      <ConfirmHost />
    </div>
  );
}
