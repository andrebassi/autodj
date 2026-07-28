"use client";

import { useGradeLoad } from "@/hooks/useGradeLoad";

/* Histograma de carga da grade nas 24h (não é waveform): cada barra = uma hora,
   altura = nº de itens agendados naquela hora. Cursor vermelho = posição atual
   no dia. Clicar numa barra salta/seleciona aquela hora (onPickHour). */
export function Timeline({
  now,
  selectedHour,
  onPickHour,
}: {
  now: Date | null;
  selectedHour?: number | null;
  onPickHour?: (hour: number) => void;
}) {
  const { hourly } = useGradeLoad();
  const hour = now?.getHours() ?? 16;
  const min = now?.getMinutes() ?? 0;
  const frac = (hour + min / 60) / 24;
  const max = Math.max(1, ...hourly);

  return (
    <div className="timeline">
      <div className="tl-track">
        {hourly.map((load, h) => {
          const h100 = 12 + (load / max) * 88; // 12%..100% da altura útil
          const cur = h === hour;
          const sel = selectedHour === h;
          return (
            <button
              key={h}
              type="button"
              className={"tl-col" + (cur ? " cur" : "") + (sel ? " sel" : "")}
              style={{ ["--h" as string]: `${h100}%` }}
              onClick={() => onPickHour?.(h)}
              title={`${String(h).padStart(2, "0")}h · ${load} ${load === 1 ? "item" : "itens"} na grade`}
            >
              <span className="tl-bar" />
              <span className="tl-lbl">{String(h).padStart(2, "0")}</span>
            </button>
          );
        })}
        <span className="tl-cursor" style={{ left: `${frac * 100}%` }} />
      </div>
    </div>
  );
}
