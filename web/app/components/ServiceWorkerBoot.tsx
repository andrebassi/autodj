"use client";

import { useEffect } from "react";

/* Registra o Service Worker (`public/sw.js`) — é ele que faz o app ABRIR sem
   servidor. Sem registro, um F5 offline mostra a página de erro do browser e
   nenhum áudio guardado adianta, porque a casca do app não existe.

   Registra depois do `load` de propósito: durante o carregamento inicial a aba já
   está disputando rede pra pintar a tela e começar o áudio; o SW pode esperar
   alguns milissegundos.

   Não faz nada em servidor sem HTTPS (a API exige contexto seguro) — localhost é
   exceção da própria spec, então o dev segue funcionando. */
export function ServiceWorkerBoot() {
  useEffect(() => {
    if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;
    const register = () => {
      void navigator.serviceWorker.register("/sw.js").catch(() => {
        /* bloqueado por política/modo privado: o app segue online-only */
      });
    };
    if (document.readyState === "complete") register();
    else window.addEventListener("load", register, { once: true });
  }, []);
  return null;
}
