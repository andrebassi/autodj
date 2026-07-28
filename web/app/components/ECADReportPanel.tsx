"use client";

import { useECADReport } from "@/hooks/useECADReport";
import { fmtDur } from "@/lib/format";
import { PLAYLOG_PERIODS, type ECADLineDTO } from "@/lib/api";

/* ECADReportPanel — the ECAD execution report for a rolling window (dia/semana/mês).
   It is a read-only projection over the immutable play log: only ECAD-relevant plays
   are counted (DRM Spotify never appears), aggregated by track into obra/intérprete +
   execuções + tempo total. Because the log is append-only, regenerating always agrees —
   what makes it trustworthy for royalty reporting. Data comes from GET /reports/ecad. */
export function ECADReportPanel({ onClose }: { onClose: () => void }) {
  const { period, setPeriod, report, loaded, busy, available, error, reload } =
    useECADReport();

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Relatório ECAD</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}

          {loaded && !available && (
            <fieldset className="mix-group">
              <legend>Relatório indisponível</legend>
              <p className="mix-lbl">
                O servidor não expôs o relatório ECAD. O gravador de PlayLog precisa
                estar ativo para que as execuções sejam registradas e apuradas.
              </p>
            </fieldset>
          )}

          {loaded && available && (
            <fieldset className="mix-group">
              <legend>Execuções ({period})</legend>
              <div className="mix-row">
                {PLAYLOG_PERIODS.map((p) => (
                  <button
                    key={p.key}
                    className={p.key === period ? "mix-save" : "mix-close"}
                    onClick={() => setPeriod(p.key)}
                    disabled={busy}
                  >
                    {p.label}
                  </button>
                ))}
              </div>

              {!report || report.lines.length === 0 ? (
                <p className="mix-lbl">Nenhuma execução ECAD no período.</p>
              ) : (
                <>
                  <ul className="ecad-list">
                    <li className="ecad-head">
                      <span className="ecad-title">Obra / intérprete</span>
                      <span className="ecad-plays mono">Exec.</span>
                      <span className="ecad-dur mono">Tempo total</span>
                    </li>
                    {report.lines.map((l) => (
                      <ECADRow key={l.track_id} line={l} />
                    ))}
                  </ul>
                  <div className="ecad-totals">
                    <span className="ecad-title">Total</span>
                    <span className="ecad-plays mono">{report.total_plays}×</span>
                    <span className="ecad-dur mono">
                      {fmtDur(report.total_duration_ms)}
                    </span>
                  </div>
                </>
              )}
            </fieldset>
          )}
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-save" onClick={() => void reload()} disabled={busy}>
            Atualizar ↻
          </button>
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}

/* one ECAD line: aggregated obra/intérprete with its play count and total aired time. */
function ECADRow({ line }: { line: ECADLineDTO }) {
  return (
    <li className="ecad-row">
      <span className="ecad-title">
        {line.title}
        {line.artist ? <span className="ecad-artist"> — {line.artist}</span> : null}
      </span>
      <span className="ecad-plays mono">{line.plays}×</span>
      <span className="ecad-dur mono">{fmtDur(line.total_duration_ms)}</span>
    </li>
  );
}
