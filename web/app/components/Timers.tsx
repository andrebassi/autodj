/* Bloco de timers do header (à esquerda do relógio de parede).
   Três leituras da faixa/ fila no ar + tag RDS:
     ⏱ (up)    = tempo DECORRIDO da faixa atual (quanto já tocou).
     － (minus) = tempo RESTANTE da faixa atual (countdown até acabar) —
                  é a deixa do operador p/ preparar locução/próxima entrada.
     ＋ (plus)  = RESERVA da fila (autonomia): quanto ainda toca sozinho até
                  a playlist esvaziar. Alto = pode sair; baixo = alimentar a fila.
     RDS       = indicador do RadioText/RDS (metadado no ar via encoder).

   Posição real só chega via Spotify Web Playback SDK; sem feed ao vivo
   (fila local sem SDK) os campos mostram "--:--". */

/* ms → m:ss (ou h:mm:ss se passar de 1h, caso da reserva de fila cheia). */
function clk(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = s % 60;
  const mm = h > 0 ? String(m).padStart(2, "0") : String(m);
  const head = h > 0 ? `${h}:` : "";
  return `${head}${mm}:${String(ss).padStart(2, "0")}`;
}

export function Timers({
  posMs = 0,
  durMs = 0,
  queueMs = 0,
}: {
  posMs?: number;
  durMs?: number;
  queueMs?: number;
}) {
  const live = durMs > 0; // sem duração conhecida = sem feed ao vivo
  const elapsed = live ? clk(posMs) : "--:--";
  const remain = live ? clk(Math.max(0, durMs - posMs)) : "--:--";
  const reserve = queueMs > 0 ? clk(Math.max(0, queueMs - posMs)) : "--:--";

  return (
    <div className="timers">
      <div className="timer up" title="Decorrido da faixa atual">
        <span className="t-ico">⏱</span>
        {elapsed}
      </div>
      <div className="timer plus" title="Reserva da fila (autonomia até esvaziar)">
        ＋{reserve}
      </div>
      <div className="timer minus" title="Restante da faixa atual (countdown)">
        －{remain}
      </div>
      <div className="rds" title="RDS RadioText no ar">
        RDS
      </div>
    </div>
  );
}
