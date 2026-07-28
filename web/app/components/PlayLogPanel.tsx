"use client";

import { usePlayLog } from "@/hooks/usePlayLog";
import { fmtDur } from "@/lib/format";
import { type PlayLogEntryDTO } from "@/lib/api";
import { TopPlayed } from "@/components/TopPlayed";

/* PlayLogPanel — the append-only play log (what actually aired) plus the "mais
   tocadas" ranking, over a shared rolling window (dia/semana/mês). Data comes
   entirely from GET /playlog + /playlog/top. The per-row broadcast flag
   ("irradiado / não irradiado") was dropped from the UI — irrelevant for home
   use; the broadcasted/ecad_relevant fields still exist server-side for reports. */
export function PlayLogPanel({ onClose }: { onClose: () => void }) {
  const { period, setPeriod, history, top, loaded, busy, available, error, reload } =
    usePlayLog();

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal pl-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Histórico & Mais Tocadas</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}

          {loaded && !available && (
            <fieldset className="mix-group">
              <legend>Histórico indisponível</legend>
              <p className="mix-lbl">
                O servidor não expôs o registro de tocadas. O gravador de PlayLog
                precisa estar ativo para acompanhar o histórico e as mais tocadas.
              </p>
            </fieldset>
          )}

          {loaded && available && (
            <>
              <TopPlayed items={top} period={period} onPeriod={setPeriod} busy={busy} />

              <fieldset className="mix-group">
                <legend>Histórico ({period})</legend>
                {history.length === 0 ? (
                  <p className="mix-lbl">Nada tocado no período.</p>
                ) : (
                  <ul className="pl-hist">
                    {history.map((e) => (
                      <PlayLogRow key={e.id} entry={e} />
                    ))}
                  </ul>
                )}
              </fieldset>
            </>
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

/* one history row: play time, title/artist and duration. The broadcast/ECAD flag
   was removed from the UI (not useful for home use). */
function PlayLogRow({ entry }: { entry: PlayLogEntryDTO }) {
  const at = new Date(entry.played_at);
  const time = at.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
  return (
    <li className="pl-hist-row">
      <span className="pl-hist-time mono">{time}</span>
      <span className="pl-hist-title">
        {entry.title}
        {entry.artist ? <span className="pl-hist-artist"> — {entry.artist}</span> : null}
      </span>
      <span className="pl-hist-dur mono">{fmtDur(entry.duration_ms)}</span>
    </li>
  );
}
