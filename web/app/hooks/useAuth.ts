"use client";

import { useEffect, useState } from "react";
import { fetchMe, setToken, HTTPError, type UserDTO } from "@/lib/api";

/* Sessão do operador — SÓ SONDA, não faz login (0.2.26).

   O AutoDJ caseiro é single-user por decisão de produto: a estação é isolada por
   browser (fingerprint), não por conta. `login()`, `logout()` e `can(perm)` foram
   REMOVIDOS junto com a tela de login: nenhuma tela chamava — `can()` não gateava
   um único botão — e o formulário só aparecia por acidente, quando um restart do
   servidor invalidava o token guardado.

   O backend de auth (JWT, hash, RBAC, `AUTODJ_SINGLE_USER=0`) continua inteiro de
   propósito: produção é uma URL pública, e ele é a única alavanca se um dia for
   preciso fechar o acesso. Apagar é fácil; refazer é projeto. */
/* Último operador conhecido, em localStorage. É metadado pequeno (nome + papel) —
   exatamente o que localStorage guarda bem — e existe só pra abrir o console
   quando o servidor não responde. Não é credencial: o token continua no seu lugar
   e o backend valida tudo de novo assim que voltar. */
const LAST_USER = "autodj.user.v1";

function remember(u: UserDTO): void {
  try {
    window.localStorage.setItem(LAST_USER, JSON.stringify(u));
  } catch {
    /* cota/privado — só perde o conforto offline */
  }
}

function recall(): UserDTO | null {
  try {
    const raw = window.localStorage.getItem(LAST_USER);
    return raw ? (JSON.parse(raw) as UserDTO) : null;
  } catch {
    return null;
  }
}

export function useAuth(): {
  user: UserDTO | null;
  ready: boolean; // bootstrap finished (avoids a login flash on reload)
} {
  const [user, setUser] = useState<UserDTO | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let stop = false;
    (async () => {
      // Always probe /auth/me — even without a token. In single-user (home 1:1)
      // mode the backend resolves the token-less request to the operator, so the
      // console opens with no login screen. A 401 means multi-user mode (or a
      // stale token): fall back to the LoginScreen.
      try {
        const me = await fetchMe();
        if (!stop) setUser(me);
        remember(me);
      } catch (e) {
        if (!(e instanceof HTTPError) || e.status !== 401) {
          /* SERVIDOR FORA DO AR ≠ servidor pedindo login. Sem esta separação, um F5
             offline caía na tela "o servidor está em modo multiusuário" — mensagem
             confiante e errada, justamente quando o app tinha acabado de abrir do
             cache pra continuar tocando. Aqui restauramos o último operador
             conhecido e seguimos: o console abre, o áudio guardado toca e a barra
             vermelha de conectividade explica o resto. */
          const last = recall();
          if (!stop) {
            if (last) setUser(last);
            setReady(true);
          }
          return;
        }
        /* 401 pode ser token VELHO, não ausência de sessão — e a diferença decide
           se a tela de login aparece. O segredo JWT de dev é regerado a cada boot
           do servidor, então todo restart invalida o token guardado; a sonda ia com
           ele, levava 401 e caíamos no LoginScreen mesmo em modo single-user, onde
           login não existe ("que tela é essa que nem deveria existir?").
           Agora: limpa o token e SONDA DE NOVO sem ele. Se passar, é single-user e
           o console abre direto; se der 401 outra vez, aí sim é multiusuário de
           verdade e a tela de login é legítima. */
        setToken("");
        try {
          const me = await fetchMe();
          if (!stop) setUser(me);
          remember(me);
        } catch {
          /* multiusuário mesmo — segue pro login */
        }
      } finally {
        if (!stop) setReady(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  return { user, ready };
}
