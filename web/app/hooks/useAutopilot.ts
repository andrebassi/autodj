"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchAutopilot,
  setAutopilot,
  type AutopilotStatus,
} from "@/lib/api";

const EMPTY: AutopilotStatus = { enabled: false, queueLen: 0, lastTick: "" };

/* PREFERÊNCIA DE AUTO — ligado por padrão em toda sessão (0.2.27).

   O pedido: "sempre ativo por padrão quando inicia ou continua uma sessão, até o
   user não querer mais e desativar". Um AutoDJ que abre parado não é um AutoDJ.

   O detalhe que fazia isso falhar: o **STOP mestre desliga o autopilot** — e tem
   de desligar mesmo, senão o piloto religa a estação no instante seguinte e o
   botão de parar não para nada. Só que esse desligamento é uma ação de
   TRANSPORTE, não uma opinião sobre o AUTO; guardá-lo como preferência fazia a
   sessão seguinte abrir com AUTO OFF sem ninguém ter pedido.

   Então a preferência grava **apenas o clique no próprio chip**:
   - sem registro (1ª vez) ou registro "on"  → liga sozinho no boot;
   - registro "off" (você desligou no chip)  → fica desligado, e continua assim
     nas próximas sessões até você ligar de novo. */
const PREF = "autodj.auto.v1";

function prefSaysOff(): boolean {
  try {
    return window.localStorage.getItem(PREF) === "off";
  } catch {
    return false; // sem storage: o padrão é ligado
  }
}

function savePref(on: boolean): void {
  try {
    window.localStorage.setItem(PREF, on ? "on" : "off");
  } catch {
    /* modo privado/cota: perde só a memória entre sessões */
  }
}

/* polls GET /api/v1/autopilot and exposes a toggle() that POSTs the flag.
   Enabling ticks once immediately server-side. Falls back to a disabled status
   while the backend is offline so the toggle still renders. `intervalMs` sets
   the refresh cadence (default 3s) so queueLen/lastTick stay live. */
export function useAutopilot(intervalMs = 3000): {
  status: AutopilotStatus;
  loaded: boolean;
  busy: boolean;
  error: string | null;
  /** clique do OPERADOR no chip — grava a preferência da sessão */
  toggle: (enabled: boolean) => Promise<void>;
  /** desligamento por TRANSPORTE (STOP mestre) — não mexe na preferência */
  toggleForTransport: (enabled: boolean) => Promise<void>;
} {
  const [status, setStatus] = useState<AutopilotStatus>(EMPTY);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // liga-no-boot roda UMA vez por carga de página (o load abaixo é um poll de 3s).
  const bootArmed = useRef(false);

  useEffect(() => {
    let stop = false;
    const load = async () => {
      try {
        const s = await fetchAutopilot();
        if (stop || !s) return;
        setStatus(s);
        // 1ª resposta boa do servidor: aplica o padrão da sessão.
        if (!bootArmed.current) {
          bootArmed.current = true;
          if (!s.enabled && !prefSaysOff()) {
            try {
              const on = await setAutopilot(true);
              if (!stop) setStatus(on);
            } catch {
              /* servidor recusou/caiu: fica como está, sem alarme */
            }
          }
        }
      } catch {
        /* backend offline → keep last/empty status */
      } finally {
        if (!stop) setLoaded(true);
      }
    };
    load();
    const id = setInterval(load, intervalMs);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, [intervalMs]);

  const apply = useCallback(async (enabled: boolean, remember: boolean): Promise<void> => {
    setBusy(true);
    setError(null);
    try {
      const s = await setAutopilot(enabled);
      setStatus(s);
      if (remember) savePref(enabled);
    } catch (e) {
      setError(e instanceof Error ? e.message : "erro no piloto automático");
    } finally {
      setBusy(false);
    }
  }, []);

  const toggle = useCallback((enabled: boolean) => apply(enabled, true), [apply]);
  const toggleForTransport = useCallback(
    (enabled: boolean) => apply(enabled, false),
    [apply],
  );

  return { status, loaded, busy, error, toggle, toggleForTransport };
}
