"use client";

import { useEffect, useRef, useState } from "react";
import { toast, dismissToast } from "@/lib/toast";
import { isAudioAlive, setServerLinked } from "@/lib/onAir";
import { hasMirroredSnapshot } from "@/hooks/useNowPlaying";
import { BUFFERED_AHEAD } from "@/lib/audioBuffer";

/** Chave do toast fixo de rede — repetir substitui, não empilha. */
const KEY = "net";

/* Quanto tempo a queda precisa durar pra virar aviso. O WS reconecta a cada 1,5s;
   um blip de meio segundo trocando de rede não pode acender alarme no estúdio. */
const GRACE_MS = 2500;

/* Espera antes do reload quando a rede volta. 5s é o PISO de leitura: o aviso da volta
   é o único toast cuja vida é limitada por um reload (os demais expiram por ttl, todos
   ≥ 7s), e com 1,5s ele piscava — a tela recarregava antes do operador entender por quê.
   Também dá folga pro WS reconectar antes do reload. */
const RELOAD_DELAY_MS = 5000;

/* useConnectivity observa a conexão e cuida das duas pontas: avisa quando cai e
 * recarrega quando volta.
 *
 * Por que o sinal principal é o WebSocket e não `navigator.onLine`: o app é 100%
 * online contra ESTE servidor. `navigator.onLine` só diz que existe uma interface
 * de rede — dá `true` em portal cativo de hotel, em Wi-Fi sem saída e com o servidor
 * fora do ar. O WS caído é o que de fato importa: sem ele não há fila, nem
 * now-playing, nem avanço de faixa. `navigator.onLine === false` entra como sinal
 * auxiliar, porque aí a certeza é imediata e não precisa esperar o timeout do socket.
 *
 * Reload na volta é o que o operador pediu: em queda longa o playout no browser
 * perde o passo (token do Spotify expira, o IFrame do YouTube fica órfão, a fila
 * diverge do servidor) e recarregar é o caminho curto pro áudio voltar. Só recarrega
 * se a queda passou de GRACE_MS — reload é caro (corta o que estiver tocando), então
 * blip não paga esse preço. */
export function useConnectivity(wsConnected: boolean): void {
  // false = nunca esteve no ar ainda (boot). Não avisa "caiu" antes da 1ª conexão,
  // senão todo carregamento de página começa com alarme vermelho.
  const everConnected = useRef(false);
  const downSince = useRef<number | null>(null);
  const warned = useRef(false);
  const reloading = useRef(false);
  // `online`/`offline` do browser precisam re-rodar o efeito INTEIRO, não só acender
  // o aviso. Sem isto, uma oscilação de internet que não derruba o WS acendia o toast
  // e nunca o apagava (a dispensa mora no corpo do efeito, que só reagia a
  // `wsConnected`) — o operador ficava com "sem internet" preso na tela com tudo
  // funcionando. Pego ao simular `navigator.onLine = false` com o WS vivo.
  const [netTick, setNetTick] = useState(0);
  // O timer do reload NÃO pode viver no escopo do efeito: o cleanup roda a cada
  // re-execução (e `netTick` garante várias), e `clearTimeout` cancelaria o reload
  // agendado. Em ref ele sobrevive às re-execuções; `reloading` impede agendar duas
  // vezes. Antes do netTick isso passava despercebido porque o efeito re-rodava uma
  // única vez — o reload funcionava por sorte de timing.
  const reloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (wsConnected) everConnected.current = true;
    // publica o enlace pros motores de playout: sem servidor, ninguém avança a
    // fila (senão a queda troca a faixa que o operador está ouvindo).
    setServerLinked(wsConnected && navigator.onLine);
  }, [wsConnected, netTick]);

  useEffect(() => {
    let graceTimer: ReturnType<typeof setTimeout> | undefined;

    const offline = () => !navigator.onLine || !wsConnected;

    const showDown = () => {
      warned.current = true;
      /* A frase tem de dizer o que é VERDADE agora — e isso mudou duas vezes.
         Era "áudio e fila parados" quando queda = silêncio. Virou "a fila não
         avança" quando o áudio passou a sobreviver. Hoje nenhuma das duas serve: a
         fila ANDA offline (avanço no espelho, dívida paga na volta) e a música
         segue do cache.
         O limite real passou a ser o ESTOQUE: toca enquanto durarem as faixas
         guardadas (a do ar + as 3 próximas). É isso que o operador precisa saber
         pra decidir se espera ou se age. Nível de AVISO, não de erro, quando há
         som: não é falha, é modo degradado. */
      const playing = isAudioAlive();
      toast.sticky(
        playing ? "warning" : "error",
        // curtas de propósito: a barra é de UMA linha (ver .ant-alert-title no CSS).
        playing
          ? `Sem servidor — tocando do cache: esta faixa e as ${BUFFERED_AHEAD} próximas da fila.`
          : navigator.onLine
            ? `Sem conexão com o servidor — só tocam as ${BUFFERED_AHEAD} próximas guardadas.`
            : `Sem internet — só tocam as ${BUFFERED_AHEAD} próximas guardadas.`,
        KEY,
      );
    };

    // `everConnected` evita alarme antes da 1ª conexão. Mas quando o app abriu com
    // a estação ESPELHADA (F5 sem servidor), a tela parece normal — calar aqui
    // faria o operador achar que está tudo certo. Então o espelho também autoriza
    // o aviso.
    if (offline() && (everConnected.current || hasMirroredSnapshot())) {
      if (downSince.current === null) downSince.current = performance.now();
      if (!warned.current) {
        const elapsed = performance.now() - (downSince.current ?? 0);
        graceTimer = setTimeout(showDown, Math.max(0, GRACE_MS - elapsed));
      }
    } else if (!offline()) {
      const wasDown = downSince.current !== null;
      const longEnough = wasDown && performance.now() - (downSince.current ?? 0) >= GRACE_MS;
      downSince.current = null;

      if (warned.current) {
        dismissToast(KEY);
        warned.current = false;
      }
      /* Só recarrega se a queda foi longa O BASTANTE pra ter estragado o playout —
         E se o áudio NÃO atravessou a queda tocando.

         Faixa local bufferizada (`lib/audioBuffer.ts`) segue no ar mesmo com o
         servidor fora: recarregar ali seria cortar justamente o que sobreviveu, que
         é o oposto do pedido ("continuar de onde parou"). Quando o áudio venceu a
         queda, o servidor volta sozinho pelo WS e a fila se realinha no próximo
         avanço — sem reload, sem buraco no ar. */
      if (longEnough && !reloading.current && !isAudioAlive()) {
        reloading.current = true;
        toast.sticky("success", "Conexão de volta — recarregando para retomar o áudio…", KEY);
        reloadTimer.current = setTimeout(() => window.location.reload(), RELOAD_DELAY_MS);
      } else if (longEnough && !reloading.current) {
        toast.success("Conexão de volta — o áudio não parou.");
      }
    }

    // `online`/`offline` do browser são o gatilho rápido: sem eles a descoberta
    // esperaria o próximo ciclo de reconexão do WS. Re-roda o efeito inteiro (não só
    // acende) pra que a VOLTA também seja tratada — ver comentário do netTick.
    const bump = () => setNetTick((t) => t + 1);
    window.addEventListener("offline", bump);
    window.addEventListener("online", bump);
    return () => {
      // graceTimer PODE morrer aqui (é reagendado na próxima execução com o tempo já
      // decorrido descontado). O reloadTimer NÃO — ver o comentário do ref acima.
      clearTimeout(graceTimer);
      window.removeEventListener("offline", bump);
      window.removeEventListener("online", bump);
    };
  }, [wsConnected, netTick]);
}
