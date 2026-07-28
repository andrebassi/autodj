"use client";

import { useSyncExternalStore } from "react";
import {
  isServerLinked,
  serverLinkedServerSnapshot,
  subscribeServerLinked,
} from "@/lib/onAir";

/* "O servidor está alcançável agora?" para a UI.

   Existe porque, sem servidor, boa parte da tela só pode falhar: buscar no
   Spotify/YouTube, importar e enviar arquivo dependem da API. Com o cache local
   (0.2.27) o app continua aberto e tocando, então esses controles ficam ali,
   convidativos — e cada clique vira um erro. Melhor apagá-los enquanto não há
   enlace e reacendê-los sozinhos quando voltar.

   O que NÃO depende de rede continua vivo de propósito: a lista de Músicas (vem do
   espelho local) e o transporte do que já está em memória. */
export function useServerLinked(): boolean {
  return useSyncExternalStore(
    subscribeServerLinked,
    isServerLinked,
    serverLinkedServerSnapshot,
  );
}
