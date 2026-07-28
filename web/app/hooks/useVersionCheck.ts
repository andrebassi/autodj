"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { apiBase } from "@/lib/api";
import { APP_VERSION, shouldOfferReload } from "@/lib/version";
import { toast } from "@/lib/toast";
import { isPlayingNow } from "@/lib/onAir";

/* Intervalo do heartbeat. 5 min é folgado de propósito: o caso real é a aba que passa
   HORAS aberta no estúdio, não o operador que quer saber do deploy em 10 segundos. */
const POLL_MS = 5 * 60 * 1000;

/* useVersionCheck detecta que o servidor foi trocado embaixo de uma aba já aberta —
   o único buraco de staleness que o Cache-Control (v0.2.18) não fecha, porque a aba
   nunca refaz a requisição do documento.
 *
 * Consulta GET /api/v1/health (que reporta a versão do binário) no intervalo E a cada
 * volta de foco. O foco é o gatilho que mais importa: o operador tende a voltar pra aba
 * logo depois do deploy, e aí a descoberta é imediata em vez de esperar o timer.
 *
 * ⚠️ RECARREGA SOZINHO DESDE A 0.2.35, e isso REVERTE a regra anterior ("nunca
 * recarregar sozinho, porque reload derruba o áudio no ar"). Foi decisão explícita
 * do operador: versão nova tem de entrar, e um crachá que espera clique fica dias
 * sem ser notado.
 *
 * O que torna a reversão aceitável é a RETOMADA (0.2.26): a posição de cada faixa é
 * salva continuamente, então o reload volta tocando de onde parou. O corte existe —
 * são alguns segundos de silêncio — mas não se perde o lugar.
 *
 * Os 5 s de aviso não são enfeite: é a janela para o operador que estiver no meio de
 * uma locução ver o que vai acontecer.
 *
 * ⚠️ COM MÚSICA NO AR, SÓ INFORMA — O RELOAD ESPERA (0.2.41). Retomar de onde parou
 * não é o mesmo que não interromper: são segundos de silêncio no meio da faixa, e o
 * crossfade da virada, a janela de mixagem e uma locução em curso não voltam do
 * localStorage. Então o aviso aparece na hora (o operador precisa saber que há versão
 * nova) e o reload fica ARMADO, disparando no primeiro momento de silêncio.
 *
 * Adiar não é o mesmo que desistir: o crachá de clique sozinho foi justamente o que
 * não funcionou. Por isso, enquanto espera, a checagem vigia o playout de perto
 * (`SILENCE_POLL_MS`) em vez de esperar o heartbeat de 5 min — a estação pode parar
 * 10 s depois do deploy, e reagir só no próximo tique desperdiçaria a janela.
 *
 * "Tocando" vem de `isPlayingNow` (publicado por `page.tsx`), que cobre os TRÊS
 * motores. `isAudioAlive` NÃO serve aqui: ela é registrada só pelo motor local, e
 * ler a errada recarregaria a página no meio de uma faixa do Spotify. */
const RELOAD_DELAY_MS = 5000;
const SILENCE_POLL_MS = 3000;
export function useVersionCheck(): {
  updateAvailable: boolean;
  serverVersion: string | null;
} {
  const [serverVersion, setServerVersion] = useState<string | null>(null);
  // uma vez detectado, não volta atrás: se o servidor oscilar (rolling deploy com duas
  // máquinas, rede instável), o aviso sumir na cara do operador seria pior que ficar.
  const [updateAvailable, setUpdateAvailable] = useState(false);
  const seen = useRef(false);
  /* Reload armado esperando silêncio. O timer mora em REF de propósito: o cleanup do
     efeito roda a cada re-execução e um `clearTimeout` ali cancelaria o reload já
     agendado — foi exatamente esse o bug do reload por volta de rede (v0.2.21). */
  const armed = useRef(false);
  const reloadTimer = useRef<number | null>(null);

  const armReload = useCallback((v: string | null) => {
    if (armed.current) return;
    armed.current = true;
    /* Mesma `key` do aviso anterior ⇒ SUBSTITUI em vez de empilhar: o operador que
       esperou a faixa acabar vê o mesmo aviso virar contagem, não dois avisos. */
    toast.sticky(
      "info",
      `Versão nova publicada (${v ?? "?"}) — recarregando em 5 segundos para atualizar.`,
      "app-update",
    );
    reloadTimer.current = window.setTimeout(
      () => window.location.reload(),
      RELOAD_DELAY_MS,
    );
  }, []);

  const check = useCallback(async () => {
    if (seen.current) return;
    try {
      // apiBase() é só a ORIGEM (vazia em same-origin) — o /api/v1 é do chamador.
      const r = await fetch(`${apiBase()}/api/v1/health`, {
        cache: "no-store",
        // health é global (não por tenant) e a checagem não pode falhar por sessão
        headers: { Accept: "application/json" },
      });
      if (!r.ok) return;
      const body = (await r.json()) as { version?: string };
      const v = body?.version ?? null;
      setServerVersion(v);
      if (shouldOfferReload(APP_VERSION, v)) {
        seen.current = true;
        setUpdateAvailable(true);
        /* Toast FIXO (ttl 0), não temporizado: ele tem de continuar na tela até o
           reload acontecer. Um aviso que some antes do evento que anuncia é pior
           que nenhum aviso. */
        if (isPlayingNow()) {
          // com som no ar: informa AGORA, recarrega depois. O texto diz o que vai
          // acontecer e quando — "atualização disponível" sozinho deixaria o operador
          // sem saber se precisa fazer algo.
          toast.sticky(
            "info",
            `Versão nova publicada (${v ?? "?"}) — não vou interromper a música. A página recarrega sozinha assim que a reprodução parar.`,
            "app-update",
          );
        } else {
          armReload(v);
        }
      }
    } catch {
      // rede caiu / servidor reiniciando: silêncio. Erro de heartbeat não é evento
      // do operador, e transformar isso em aviso vira ruído no estúdio.
    }
  }, []);

  useEffect(() => {
    void check();
    const t = setInterval(() => void check(), POLL_MS);
    const onFocus = () => {
      if (document.visibilityState === "visible") void check();
    };
    document.addEventListener("visibilitychange", onFocus);
    window.addEventListener("focus", onFocus);
    return () => {
      clearInterval(t);
      document.removeEventListener("visibilitychange", onFocus);
      window.removeEventListener("focus", onFocus);
    };
  }, [check]);

  /* Vigia do silêncio: só existe DEPOIS de detectar versão nova e enquanto o reload
     não foi armado. Fora dessa janela não há intervalo rodando — o custo é zero na
     esmagadora maioria do tempo, que é justamente quando não há nada a atualizar. */
  useEffect(() => {
    if (!updateAvailable) return;
    const t = setInterval(() => {
      if (armed.current) return;
      if (!isPlayingNow()) armReload(serverVersion);
    }, SILENCE_POLL_MS);
    return () => clearInterval(t);
  }, [updateAvailable, serverVersion, armReload]);

  return { updateAvailable, serverVersion };
}
