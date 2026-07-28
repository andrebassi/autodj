"use client";

import { useCallback, useSyncExternalStore } from "react";
import { instrumentalsWorking, type StemStatus } from "@/lib/api";

/* Quem está sendo separado AGORA (na fila ou rodando), por id de faixa.

   ⚠️ STORE ÚNICO NO MÓDULO, não estado por componente — e isto foi um bug real.

   A primeira versão era um `useState` por chamador. Com o transporte e a grade
   montando o hook cada um por si, o clique em PROCESSAR no transporte acendia a
   barra LÁ e a grade continuava sem saber: a instância dela tinha terminado o
   ciclo (mapa vazio encerra o poll) e ninguém a acordava. Dois relógios contando
   a mesma coisa, um deles parado.

   Aqui existe UM mapa, UM timer e N assinantes. Todo mundo vê o mesmo estado no
   mesmo instante, e `refresh()` de qualquer canto serve a todos.

   O POLL SÓ VIVE ENQUANTO HÁ TRABALHO. Intervalo permanente custaria requisição a
   cada poucos segundos, para sempre, para observar algo que acontece raramente e
   por iniciativa explícita do operador. Sem trabalho, o timer morre — e volta no
   próximo `refresh()`.

   `ready` NÃO passa por aqui: já viaja no `trackDTO` de cada faixa (o arquivo é a
   verdade e não muda sozinho). Aqui é só trabalho VIVO. */
const POLL_MS = 4000;

type Work = Record<string, StemStatus>;

const EMPTY: Work = {};
let work: Work = EMPTY;
let timer = 0;
let inFlight = false;
/* A ferramenta existe NESTA instalação? `null` = ainda não perguntamos.

   404 na rota significa que o servidor não registrou o recurso (sem `demucs`), e
   é o caso de PRODUÇÃO. A UI usa isto para NÃO OFERECER a ação: um botão que só
   pode terminar em "não está instalado" é pior que um botão ausente. */
let available: boolean | null = null;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

function poll() {
  if (inFlight) return;
  inFlight = true;
  void instrumentalsWorking()
    .then((w) => {
      const wasAvailable = available;
      available = w !== null; // null = rota inexistente (404)
      const next = w ?? EMPTY;
      if (available !== wasAvailable) emit();
      // só notifica quando MUDA: o poll roda a cada 4s e a resposta costuma ser
      // idêntica; re-renderizar a grade inteira à toa é caro e pisca a tela.
      if (JSON.stringify(next) !== JSON.stringify(work)) {
        work = next;
        emit();
      }
      // reagenda só se sobrou trabalho — é o que faz o poll morrer sozinho.
      if (Object.keys(work).length > 0) {
        timer = window.setTimeout(poll, POLL_MS);
      } else {
        timer = 0;
      }
    })
    .catch(() => {
      /* sem servidor, ou rota ausente (ferramenta não instalada): sem barra. Não
         reagenda: insistir contra um 404 seria bater para sempre numa porta que
         não existe. `refresh()` explícito volta a tentar. */
      timer = 0;
    })
    .finally(() => {
      inFlight = false;
    });
}

/** pergunta AGORA e serve todos os assinantes. Usar logo após pedir uma separação:
 *  sem isto a barra só apareceria no próximo ciclo, e o clique ficaria sem retorno. */
export function refreshStemWork() {
  window.clearTimeout(timer);
  timer = 0;
  poll();
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  // primeiro assinante acorda o ciclo: pega trabalho que já vinha de antes do F5.
  if (listeners.size === 1 && timer === 0) poll();
  return () => {
    listeners.delete(fn);
    if (listeners.size === 0) {
      window.clearTimeout(timer);
      timer = 0;
    }
  };
}

const snapshot = () => work;
// no servidor (SSR do export estático) não há trabalho a mostrar.
const serverSnapshot = () => EMPTY;

export function useStemWork(): {
  /** id → "queued" | "running" | "ready"; vazio quando não há nada */
  work: Work;
  /** a ferramenta existe nesta instalação? null = ainda não sabemos (não
   *  oferecer a ação enquanto for null seria pior: a primeira consulta leva
   *  alguns ms e o botão piscaria). */
  available: boolean | null;
  /** pergunta agora (usar logo após pedir uma separação) */
  refresh: () => void;
} {
  const w = useSyncExternalStore(subscribe, snapshot, serverSnapshot);
  const refresh = useCallback(() => refreshStemWork(), []);
  return { work: w, available, refresh };
}
