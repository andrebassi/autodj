"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { apiBase, wsURL } from "@/lib/api";
import type { NowPlaying } from "@/lib/types";

/* ÚLTIMO SNAPSHOT CONHECIDO, em localStorage.

   É o que faz um F5 SEM SERVIDOR abrir com a estação montada em vez de vazia: o
   Service Worker devolve a casca do app, o áudio vem do cache em disco, mas sem a
   fila não há o que tocar — a tela abriria com "Fila vazia" e dois decks mudos.

   Aqui localStorage é o container CERTO (ao contrário do áudio, que é grande e
   binário): o snapshot é um JSON de poucos KB, e ler síncrono no boot é justamente
   o que evita o pisca de tela vazia.

   Semanticamente é um ESPELHO, nunca a verdade: o primeiro snapshot real do
   WebSocket sobrescreve tudo. Serve pra atravessar a janela sem servidor. */
const LAST_SNAP = "autodj.snapshot.v1";

function readLastSnap(): NowPlaying | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(LAST_SNAP);
    return raw ? (JSON.parse(raw) as NowPlaying) : null;
  } catch {
    return null;
  }
}

/** true se existe estação espelhada de uma sessão anterior. O aviso de "sem
 *  conexão" usa isto pra falar já no boot offline: com a tela montada pelo
 *  espelho, ficar calado faria o operador achar que está tudo normal. */
export function hasMirroredSnapshot(): boolean {
  return readLastSnap() !== null;
}

function saveLastSnap(s: NowPlaying): void {
  try {
    window.localStorage.setItem(LAST_SNAP, JSON.stringify(s));
  } catch {
    /* cota/privado — só perde o conforto offline */
  }
}

/* live now-playing snapshot over WebSocket + POST transport helper */
export function useNowPlaying() {
  // arranca do espelho: se o servidor não responder, a estação já aparece montada.
  const [snap, setSnap] = useState<NowPlaying | null>(() => readLastSnap());
  const [connected, setConnected] = useState(false);
  const [busy, setBusy] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    let stop = false;
    let retry: ReturnType<typeof setTimeout>;
    const connect = () => {
      if (stop) return;
      let ws: WebSocket;
      try {
        ws = new WebSocket(wsURL());
      } catch {
        // A malformed URL (e.g. an unexpected page scheme) must never crash the
        // app — retry later; the console still renders from the REST snapshot.
        retry = setTimeout(connect, 1500);
        return;
      }
      wsRef.current = ws;
      ws.onopen = () => setConnected(true);
      ws.onclose = () => {
        setConnected(false);
        retry = setTimeout(connect, 1500);
      };
      ws.onerror = () => ws.close();
      ws.onmessage = (e) => {
        try {
          const fresh = JSON.parse(e.data) as NowPlaying;
          setSnap(fresh);
          saveLastSnap(fresh); // espelho pro próximo boot sem servidor
        } catch {
          /* ignore */
        }
      };
    };
    connect();
    return () => {
      stop = true;
      clearTimeout(retry);
      wsRef.current?.close();
    };
  }, []);

  /* AVANÇO OFFLINE (0.2.27).

     Com o cache local a música continua tocando sem servidor — mas a FILA é do
     servidor, então o cabeçalho ficava preso na faixa que já tinha acabado e o
     deck não trocava. Do lado do operador isso parece travamento, não modo
     degradado.

     Aqui o `next` ganha um caminho offline: adianta a fila NO ESPELHO e anota a
     dívida. Quando o servidor volta, a dívida é paga chamando `next` a mesma
     quantidade de vezes — a fila dele não mudou enquanto estava fora, então os dois
     lados terminam no mesmo lugar. A dívida é persistida porque um F5 no meio da
     queda não pode fazer o servidor voltar "atrasado" em relação ao que já tocou. */
  const NEXT = "/api/v1/playback/next";
  const DEBT = "autodj.pendingnext.v1";

  const readDebt = (): number => {
    try {
      return Number(window.localStorage.getItem(DEBT) ?? "0") || 0;
    } catch {
      return 0;
    }
  };
  const writeDebt = (n: number) => {
    try {
      if (n > 0) window.localStorage.setItem(DEBT, String(n));
      else window.localStorage.removeItem(DEBT);
    } catch {
      /* ignora */
    }
  };

  const advanceMirror = useCallback(() => {
    setSnap((cur) => {
      if (!cur?.queue || cur.queue.length < 2) return cur;
      const rest = cur.queue.slice(1);
      const next: NowPlaying = { ...cur, now_playing: rest[0].playable, queue: rest };
      saveLastSnap(next);
      return next;
    });
  }, []);

  const post = useCallback(
    async (path: string) => {
      setBusy(true);
      try {
        await fetch(apiBase() + path, { method: "POST" });
      } catch (e) {
        // Só o avanço tem caminho offline: enfileirar/mover exigem o servidor de
        // verdade (mexem no acervo), e fingir que deram certo criaria divergência
        // silenciosa.
        if (path === NEXT) {
          advanceMirror();
          writeDebt(readDebt() + 1);
        } else {
          throw e;
        }
      } finally {
        setBusy(false);
      }
    },
    [advanceMirror],
  );

  // Servidor de volta: paga a dívida de avanços feitos offline, em ordem.
  useEffect(() => {
    if (!connected) return;
    let stop = false;
    void (async () => {
      let debt = readDebt();
      while (debt > 0 && !stop) {
        try {
          await fetch(apiBase() + NEXT, { method: "POST" });
        } catch {
          return; // caiu de novo no meio: a dívida fica pra próxima volta
        }
        debt -= 1;
        writeDebt(debt);
      }
    })();
    return () => {
      stop = true;
    };
  }, [connected]);

  // enqueue inserts a catalog track into the playout queue.
  // pos<0 appends; pos=0 plays next; playNow skips the current track.
  const enqueue = useCallback(
    async (trackId: string, pos: number, playNow: boolean, origin = "") => {
      setBusy(true);
      try {
        const res = await fetch(apiBase() + "/api/v1/playback/enqueue", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ track_id: trackId, pos, play_now: playNow, origin }),
        });
        if (!res.ok) {
          // Erro raro agora: a fila aceita Spotify (hard_cut). Só o bus PCM/mix
          // real (Liquidsoap) recusa DRM. Propaga o {error} do backend.
          const body = (await res.json().catch(() => null)) as { error?: string } | null;
          const raw = body?.error ?? "";
          throw new Error(raw || `falha ao adicionar à fila (HTTP ${res.status})`);
        }
      } finally {
        setBusy(false);
      }
    },
    [],
  );

  // dequeue drops a pending queue item by mirror index (pos>=1; pos 0 is
  // now-playing and can't be dequeued — use Próxima to skip it).
  const dequeue = useCallback(async (pos: number) => {
    setBusy(true);
    try {
      const res = await fetch(apiBase() + "/api/v1/playback/dequeue", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ pos }),
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new Error(body?.error || `falha ao remover da fila (HTTP ${res.status})`);
      }
    } finally {
      setBusy(false);
    }
  }, []);

  // move reorders the running order: it moves the pending item at mirror index
  // `from` to index `to` (both >=1; index 0 is now-playing and is off-limits on
  // either end). Used by the ↑/↓ controls in the queue.
  const move = useCallback(async (from: number, to: number) => {
    setBusy(true);
    try {
      const res = await fetch(apiBase() + "/api/v1/playback/move", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ from, to }),
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new Error(body?.error || `falha ao reordenar a fila (HTTP ${res.status})`);
      }
    } finally {
      setBusy(false);
    }
  }, []);

  return { snap, connected, busy, post, enqueue, dequeue, move };
}
