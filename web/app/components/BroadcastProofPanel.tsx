"use client";

import { useBroadcastProof } from "@/hooks/useBroadcastProof";
import { fmtDur } from "@/lib/format";
import { PLAYLOG_PERIODS, type ProofLineDTO } from "@/lib/api";

/* pt-BR label for a broadcast source. Spotify is intentionally absent — a DRM
   play never airs (broadcasted=false), so it never reaches this proof. */
const SOURCE_LABEL: Record<string, string> = {
  local: "Local",
  tts: "Voz AI",
  line_in: "Line-in",
};

/* BroadcastProofPanel — the chronological irradiation proof for a rolling window
   (dia/semana/mês). It is a read-only projection over the immutable play log:
   every aired play (broadcasted=true), one row per play, in play order — the
   auditable evidence of what actually went to air. Because the log is
   append-only, regenerating always agrees. Data comes from GET /reports/broadcast. */
export function BroadcastProofPanel({ onClose }: { onClose: () => void }) {
  const { period, setPeriod, proof, loaded, busy, available, error, reload } =
    useBroadcastProof();

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Comprovante de irradiação</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}

          {loaded && !available && (
            <fieldset className="mix-group">
              <legend>Comprovante indisponível</legend>
              <p className="mix-lbl">
                O servidor não expôs o comprovante de irradiação. O gravador de
                PlayLog precisa estar ativo para registrar o que foi ao ar.
              </p>
            </fieldset>
          )}

          {loaded && available && (
            <fieldset className="mix-group">
              <legend>Irradiado ({period})</legend>
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

              {!proof || proof.lines.length === 0 ? (
                <p className="mix-lbl">Nada irradiado no período.</p>
              ) : (
                <>
                  <ul className="proof-list">
                    <li className="proof-head">
                      <span className="proof-time mono">Hora</span>
                      <span className="proof-title">Obra / intérprete</span>
                      <span className="proof-src">Fonte</span>
                      <span className="proof-dur mono">Duração</span>
                    </li>
                    {proof.lines.map((l) => (
                      <ProofRow key={l.entry_id} line={l} />
                    ))}
                  </ul>
                  <div className="proof-totals">
                    <span className="proof-time" />
                    <span className="proof-title">
                      Total — {proof.total_entries} execuç
                      {proof.total_entries === 1 ? "ão" : "ões"}
                    </span>
                    <span className="proof-src" />
                    <span className="proof-dur mono">
                      {fmtDur(proof.total_duration_ms)}
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

/* one proof row: exact air time, obra/intérprete, source and duration. */
function ProofRow({ line }: { line: ProofLineDTO }) {
  const at = new Date(line.played_at);
  const time = at.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
  return (
    <li className="proof-row">
      <span className="proof-time mono">{time}</span>
      <span className="proof-title">
        {line.title}
        {line.artist ? <span className="proof-artist"> — {line.artist}</span> : null}
      </span>
      <span className="proof-src">{SOURCE_LABEL[line.source] ?? line.source}</span>
      <span className="proof-dur mono">{fmtDur(line.duration_ms)}</span>
    </li>
  );
}
