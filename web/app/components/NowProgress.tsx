"use client";

/* Linha de progresso REAL do no-ar, na faixa do cabeçalho (mesma ideia da barra
   dos decks). Preenche pela posição real da faixa (Spotify via SDK: posMs/durMs):
   mostra quanto já tocou (esquerda) e quanto falta (direita). Sem amplitude —
   fonte DRM não expõe PCM; só progresso é honesto. */
/* Formato legível p/ humano: >= 1h vira "1h20m" (some com os segundos, que a
   essa escala não importam); abaixo de 1h fica "MM:SS" como sempre. */
function fmt(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h${String(m).padStart(2, "0")}m`;
  return `${String(m).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}

export function NowProgress({
  posMs = 0,
  durMs = 0,
  active = false,
}: {
  posMs?: number; // posição real (ms)
  durMs?: number; // duração real (ms)
  active?: boolean; // há feed real dirigindo (senão barra vazia)
}) {
  const dur = durMs > 0 ? durMs : 0;
  const pos = dur > 0 ? Math.min(Math.max(0, posMs), dur) : Math.max(0, posMs);
  const frac = active && dur > 0 ? pos / dur : 0;
  const pct = Math.max(0, Math.min(1, frac)) * 100;

  return (
    <div className="npv">
      <span className="npv-t npv-elapsed">{active ? fmt(pos) : "00:00"}</span>
      <div className="npv-track">
        <span className="npv-fill" style={{ width: `${pct}%` }} />
        {active && pct > 0 && <span className="npv-head" style={{ left: `${pct}%` }} />}
      </div>
      <span className="npv-t npv-remain">{active && dur > 0 ? "-" + fmt(dur - pos) : "--:--"}</span>
    </div>
  );
}
