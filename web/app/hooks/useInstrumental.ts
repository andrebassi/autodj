"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { instrumentalStatus, prepareInstrumental, type StemStatus } from "@/lib/api";
import { toast } from "@/lib/toast";

/* Separação de vocal por IA — o lado do operador.

   É PRÉ-RENDER, não um botão ao vivo: leva ~1,4× o tempo real (medido — 45s de
   áudio em 64,8s num M1 Pro). Por isso a interface é "pede e acompanha", nunca
   uma chamada que espera resposta.

   O poll só existe ENQUANTO há trabalho em curso. Um intervalo permanente
   custaria uma requisição a cada poucos segundos por faixa, para sempre, para
   observar algo que muda uma vez na vida da faixa. */

/** de quanto em quanto tempo perguntamos o andamento. */
const POLL_MS = 3000;

export function useInstrumental(): {
  /** estado por faixa (só das que já foram pedidas nesta sessão) */
  status: Record<string, StemStatus>;
  /** a ferramenta existe nesta máquina? null = ainda não sabemos */
  available: boolean | null;
  /** pede a separação de uma faixa */
  prepare: (id: string, title?: string) => Promise<void>;
} {
  const [status, setStatus] = useState<Record<string, StemStatus>>({});
  const [available, setAvailable] = useState<boolean | null>(null);
  // ids em andamento — o poll vive só enquanto este conjunto não está vazio.
  const [pending, setPending] = useState<string[]>([]);
  const titles = useRef<Record<string, string>>({});

  const prepare = useCallback(async (id: string, title?: string) => {
    if (!id) return;
    if (title) titles.current[id] = title;
    try {
      const st = await prepareInstrumental(id);
      if (st === null) {
        // 404 = rota não registrada = ferramenta não instalada nesta máquina.
        setAvailable(false);
        toast.warning(
          "Este AutoDJ não separa vozes: o recurso exige uma ferramenta pesada, instalada à parte, e não roda no servidor. Use o AutoDJ do seu computador (instalação: scripts/build-demucs.sh).",
        );
        return;
      }
      setAvailable(true);
      setStatus((s) => ({ ...s, [id]: st }));
      if (st === "queued" || st === "running") {
        setPending((p) => (p.includes(id) ? p : [...p, id]));
        toast.info(
          `Preparando o instrumental de “${titles.current[id] ?? "faixa"}”. Leva alguns minutos — pode continuar usando.`,
        );
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Falha ao pedir a separação.");
    }
  }, []);

  useEffect(() => {
    if (pending.length === 0) return;
    let stop = false;
    const id = setInterval(async () => {
      for (const trackID of pending) {
        try {
          const r = await instrumentalStatus(trackID);
          if (stop || !r) continue;
          setStatus((s) => ({ ...s, [trackID]: r.status }));
          if (r.status === "ready" || r.status === "failed") {
            setPending((p) => p.filter((x) => x !== trackID));
            const name = titles.current[trackID] ?? "faixa";
            if (r.status === "ready") {
              toast.success(`Instrumental de “${name}” pronto.`);
            } else {
              toast.error(`Não deu para separar “${name}”: ${r.error || "falha no modelo"}`);
            }
          }
        } catch {
          /* rede/servidor: tenta de novo no próximo tick, sem alarme */
        }
      }
    }, POLL_MS);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, [pending]);

  return { status, available, prepare };
}
