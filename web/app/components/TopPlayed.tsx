"use client";

import { PLAYLOG_PERIODS, type TopItemDTO } from "@/lib/api";

/* TopPlayed — the "mais tocadas" ranking for a rolling window (dia/semana/mês).
   The period selector drives the window; each row shows rank, title/artist and the
   play count. Data comes from GET /playlog/top — no hardcoded catalog. */
export function TopPlayed({
  items,
  period,
  onPeriod,
  busy,
}: {
  items: TopItemDTO[];
  period: string;
  onPeriod: (p: string) => void;
  busy: boolean;
}) {
  return (
    <fieldset className="mix-group">
      <legend>Mais tocadas</legend>
      <div className="mix-row">
        {PLAYLOG_PERIODS.map((p) => (
          <button
            key={p.key}
            className={p.key === period ? "mix-save" : "mix-close"}
            onClick={() => onPeriod(p.key)}
            disabled={busy}
          >
            {p.label}
          </button>
        ))}
      </div>
      {items.length === 0 ? (
        <p className="mix-lbl">Sem tocadas no período.</p>
      ) : (
        <ol className="pl-rank">
          {items.map((it, i) => (
            <li key={it.track_id} className="pl-rank-row">
              <span className="pl-rank-pos mono">{i + 1}</span>
              <span className="pl-rank-title">
                {it.title}
                {it.artist ? <span className="pl-rank-artist"> — {it.artist}</span> : null}
              </span>
              <span className="pl-rank-count mono">{it.count}×</span>
            </li>
          ))}
        </ol>
      )}
    </fieldset>
  );
}
