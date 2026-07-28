"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { fadeMs } from "@/lib/fade";
import type { NowPlaying } from "@/lib/types";

/* MIXAGEM CROSS-MOTOR (v0.2.12) — crossfade DE VERDADE no fim de cada faixa,
   com os dois provedores tocando AO MESMO TEMPO.

   O problema: até aqui o fade na virada era SEQUENCIAL — a que saía descia, a
   fila avançava, e só então a entrante subia. Dois fades encostados, nunca uma
   sobreposição. Quem ouve percebe um buraco no meio, não uma mixagem.

   O que muda: `fadeMs()` ANTES do fim natural da faixa no ar, o motor da PRÓXIMA
   é iniciado localmente (`startMix`) já subindo de 0, enquanto o motor da atual
   desce a 0 (`fadeOut`). Os dois soam juntos durante a janela do fade — mixagem
   como num DJ, só que com provedores diferentes.

   A FILA NÃO É ADIANTADA. Este é o ponto que separa esta implementação da
   tentativa revertida da v0.2.6: lá o `POST /playback/next` era antecipado, o
   now-playing virava a entrante cedo e todo o resto (deck, takeover do Spotify,
   histórico) via a troca acontecer antes da hora. Aqui o servidor segue avançando
   só no fim natural da faixa que sai; a entrante toca "por fora" durante a janela
   e os hooks a ADOTAM (`preStarted`) quando o now-playing finalmente alcança ela.
   Zero mudança no servidor.

   Escopo (v0.2.26 — o local entrou):
   - **Spotify ↔ YouTube**: provedores diferentes, cada um com seu motor;
   - **local ↔ local**: DOIS `<audio>` em ping-pong, o ocioso já bufferizado pela
     pré-carga (v0.2.11) — é o caso mais comum de quem toca o próprio acervo e era
     justamente o que ficava no corte seco;
   - **spotify→spotify continua impossível** (uma conta = um playback ativo), e
     local↔streaming segue seco: o servidor só entrega o arquivo da faixa no ar,
     então não há o que sobrepor sem adiantar a fila;
   - mesma faixa nos dois decks nunca mixa (checagem de uri). */

export type MixEngine = {
  /** põe a uri no ar AGORA subindo de 0 — por cima do outro motor */
  startMix: (uri: string) => boolean;
  /** desce o ganho da faixa que está saindo até 0 */
  fadeOut: () => Promise<void>;
};

/* estado da janela p/ a UI: o servidor ainda diz que o now-playing é a faixa que
   sai, então SEM isto a tela mostra a entrante parada em 00:00 com selo CUE
   enquanto ela já está tocando ("o áudio mixou mas a tela não refletiu"). Os decks
   consomem este estado p/ rodar o tempo da entrante e marcar as duas como MIX. */
export type CrossMixState = {
  active: boolean;
  /** uri da entrante que já está no ar (casa com `queue[1]`) */
  uri: string;
  /** quanto já correu da entrante (aprox. — conta do `startMix`) */
  elapsedMs: number;
  /** duração da janela do fade (⚙) */
  ms: number;
};

const IDLE: CrossMixState = { active: false, uri: "", elapsedMs: 0, ms: 0 };
/* passo do relógio da UI durante a janela: 250ms basta p/ a barra/timer da
   entrante andarem lisos e só roda nos poucos segundos da mixagem. */
const UI_TICK_MS = 250;

/* "local" é o catch-all do playout (tudo que não é out-of-band, mesma regra do
   `useLocalPlayout`) — line_in/tts caem aqui e tocam pelo `<audio>` como qualquer PCM. */
const isLocalSrc = (s: string): boolean => s !== "spotify" && s !== "youtube";

/* Par sobreponível? Precisa de DOIS caminhos de áudio soando ao mesmo tempo:
   - **motores DIFERENTES** (local↔spotify, local↔youtube, spotify↔youtube e os
     inversos): cada faixa no seu motor, sobreposição natural;
   - **local↔local**: os dois `<audio>` do ping-pong, com o ocioso já bufferizado
     pela pré-carga (v0.2.11).
   Fora:
   - **spotify↔spotify** — uma conta = UM playback ativo; pôr a próxima no ar mata
     a atual. Impossível por construção da plataforma.
   - **youtube↔youtube** — as duas instâncias do IFrame existem, mas hoje o
     `startMix` do motor dá `standby()` (pausa) na que está no ar assumindo que a
     faixa de saída é de OUTRO motor, e o `fadeOut` mira sempre a "ao vivo" — que
     depois do swap já é a entrante. Sobrepor no mesmo motor exige ramp POR
     INSTÂNCIA (como o local ganhou aqui). Até lá, esse par segue em corte seco. */
const canMix = (a: string, b: string): boolean =>
  (isLocalSrc(a) && isLocalSrc(b)) || (a !== b && !(a === "youtube" && b === "youtube"));

/* margem depois do fade: faixa curta demais (ou fade longo demais p/ ela) não
   mixa — começar a próxima quase junto com o início da atual seria um atropelo,
   não uma transição. */
const MIN_HEADROOM_MS = 3000;
/* tarde demais: com menos que isto sobrando não dá pra sobrepor nada — o
   `startMix` chegaria depois do fim natural. */
const TOO_LATE_MS = 600;

export function useCrossEngineMix(opts: {
  snap: NowPlaying | null;
  wantPlay: boolean;
  playing: boolean;
  posMs: number;
  durMs: number;
  spotify: MixEngine;
  youtube: MixEngine;
  local: MixEngine;
}): CrossMixState {
  const { snap, wantPlay, playing, posMs, durMs, spotify, youtube, local } = opts;

  const np = snap?.now_playing;
  const npUri = np?.uri ?? "";
  const npSource = np?.source ?? "";
  const npDeclaredDur = np?.duration_ms ?? 0;
  const next = snap?.queue?.[1]?.playable;
  const nextUri = next?.uri ?? "";
  const nextSource = next?.source ?? "";

  // engines em ref: as funções mudam de identidade a cada render dos hooks de
  // playout e não devem re-disparar o efeito (que é armado por posição).
  const eng = useRef({ spotify, youtube, local });
  eng.current = { spotify, youtube, local };
  const engineFor = (source: string): MixEngine =>
    source === "spotify"
      ? eng.current.spotify
      : source === "youtube"
        ? eng.current.youtube
        : eng.current.local;

  // 1 mixagem por faixa no ar. Zera quando o now-playing muda.
  const mixedFor = useRef("");
  // janela aberta (p/ a UI). Fecha quando o now-playing muda — nesse ponto ou o
  // servidor alcançou a entrante (o deck dela vira o ao vivo de verdade) ou o
  // operador pulou pra outra faixa.
  const [win, setWin] = useState<{ uri: string; startedAt: number; ms: number } | null>(
    null,
  );
  useEffect(() => {
    mixedFor.current = "";
    setWin(null);
  }, [npUri]);
  useEffect(() => {
    if (!wantPlay) setWin(null);
  }, [wantPlay]);

  useEffect(() => {
    if (!wantPlay || !playing || !npUri || !nextUri) return;
    if (mixedFor.current === npUri) return;
    if (!canMix(npSource, nextSource)) return;
    if (npUri === nextUri) return; // mesma faixa nos dois decks → nunca mixa
    const ms = fadeMs();
    if (ms <= 0) return; // fade desligado no ⚙ → corte seco, como antes
    // duração real do motor; se ainda não chegou, a declarada do catálogo.
    const dur = durMs > 0 ? durMs : npDeclaredDur;
    if (dur <= ms + MIN_HEADROOM_MS) return;
    const remaining = dur - posMs;
    if (remaining > ms || remaining < TOO_LATE_MS) return;

    mixedFor.current = npUri;
    const incoming = engineFor(nextSource);
    const outgoing = engineFor(npSource);
    // entrante primeiro: se o motor não conseguir começar (device/iframe não
    // pronto), NÃO baixamos o volume do que está no ar — melhor sem mixagem do
    // que com silêncio.
    if (!incoming.startMix(nextUri)) {
      mixedFor.current = "";
      return;
    }
    setWin({ uri: nextUri, startedAt: Date.now(), ms });
    void outgoing.fadeOut();
  }, [
    wantPlay,
    playing,
    npUri,
    npSource,
    npDeclaredDur,
    nextUri,
    nextSource,
    posMs,
    durMs,
  ]);

  // relógio só enquanto a janela está aberta (poucos segundos por faixa).
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!win) return;
    const id = setInterval(() => setTick((t) => t + 1), UI_TICK_MS);
    return () => clearInterval(id);
  }, [win]);

  return useMemo(
    () =>
      win
        ? {
            active: true,
            uri: win.uri,
            elapsedMs: Math.max(0, Date.now() - win.startedAt),
            ms: win.ms,
          }
        : IDLE,
    // `tick` entra de propósito: é ele que faz o elapsed andar na tela.
    [win, tick],
  );
}
