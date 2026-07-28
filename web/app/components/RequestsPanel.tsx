"use client";

import { useState } from "react";
import { useRequests } from "@/hooks/useRequests";
import { REQUEST_STATUSES, type RequestDTO } from "@/lib/api";

/* RequestsPanel — the listener request / dedication moderation queue (Story 8.3).
   The top form records a new pending request (ouvinte + música + dedicatória); the
   list below shows the queue for the selected status filter, with the moderation
   actions each request allows. The lifecycle is enforced server-side (pending →
   approved/rejected; approved → played/rejected), so buttons only appear for legal
   transitions. This is metadata, not audio — independent of the two-engine rule.
   Data comes from GET/POST /requests. */
export function RequestsPanel({ onClose }: { onClose: () => void }) {
  const { filter, setFilter, items, loaded, busy, available, error, reload, submit, moderate } =
    useRequests();
  const [listener, setListener] = useState("");
  const [song, setSong] = useState("");
  const [dedication, setDedication] = useState("");

  async function onSubmit() {
    const ok = await submit(listener, song, dedication);
    if (ok) {
      setListener("");
      setSong("");
      setDedication("");
    }
  }

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Pedidos &amp; dedicatórias</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}

          {loaded && !available && (
            <fieldset className="mix-group">
              <legend>Fila indisponível</legend>
              <p className="mix-lbl">
                O servidor não expôs a fila de pedidos. O serviço de pedidos precisa
                estar ativo para receber e moderar as solicitações dos ouvintes.
              </p>
            </fieldset>
          )}

          {loaded && available && (
            <>
              <fieldset className="mix-group">
                <legend>Novo pedido</legend>
                <div className="mix-row">
                  <label className="mix-lbl">Ouvinte</label>
                  <input
                    className="req-input"
                    value={listener}
                    onChange={(e) => setListener(e.target.value)}
                    placeholder="Nome do ouvinte"
                    disabled={busy}
                  />
                </div>
                <div className="mix-row">
                  <label className="mix-lbl">Música</label>
                  <input
                    className="req-input"
                    value={song}
                    onChange={(e) => setSong(e.target.value)}
                    placeholder="Título — intérprete"
                    disabled={busy}
                  />
                </div>
                <div className="mix-row">
                  <label className="mix-lbl">Dedicatória</label>
                  <input
                    className="req-input"
                    value={dedication}
                    onChange={(e) => setDedication(e.target.value)}
                    placeholder="Opcional"
                    disabled={busy}
                  />
                </div>
                <div className="mix-row">
                  <button className="mix-save" onClick={() => void onSubmit()} disabled={busy}>
                    Enviar pedido ✉
                  </button>
                </div>
              </fieldset>

              <fieldset className="mix-group">
                <legend>Fila</legend>
                <div className="mix-row">
                  <button
                    className={filter === "" ? "mix-save" : "mix-close"}
                    onClick={() => setFilter("")}
                    disabled={busy}
                  >
                    Todos
                  </button>
                  {REQUEST_STATUSES.map((s) => (
                    <button
                      key={s.key}
                      className={s.key === filter ? "mix-save" : "mix-close"}
                      onClick={() => setFilter(s.key)}
                      disabled={busy}
                    >
                      {s.label}
                    </button>
                  ))}
                </div>

                {items.length === 0 ? (
                  <p className="mix-lbl">Nenhum pedido no filtro.</p>
                ) : (
                  <ul className="req-list">
                    {items.map((r) => (
                      <RequestRow
                        key={r.id}
                        req={r}
                        busy={busy}
                        onModerate={(action) => void moderate(r.id, action)}
                      />
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

/* STATUS_LABEL maps the domain status to a pt-BR badge label. */
const STATUS_LABEL: Record<string, string> = {
  pending: "Pendente",
  approved: "Aprovado",
  rejected: "Rejeitado",
  played: "Tocado",
};

/* one queued request: the listener/song/dedication plus the moderation actions its
   current status allows. pending → aprovar/rejeitar; approved → tocar/rejeitar;
   terminal states (rejected/played) show no actions. */
function RequestRow({
  req,
  busy,
  onModerate,
}: {
  req: RequestDTO;
  busy: boolean;
  onModerate: (action: "approve" | "reject" | "played") => void;
}) {
  return (
    <li className="req-row">
      <div className="req-meta">
        <span className={"req-badge req-" + req.status}>
          {STATUS_LABEL[req.status] ?? req.status}
        </span>
        <span className="req-song">{req.song_hint}</span>
        <span className="req-listener">— {req.listener_name}</span>
      </div>
      {req.dedication ? <div className="req-dedication">“{req.dedication}”</div> : null}
      <div className="req-actions">
        {req.status === "pending" && (
          <>
            <button className="mix-save" onClick={() => onModerate("approve")} disabled={busy}>
              Aprovar ✓
            </button>
            <button className="mix-close" onClick={() => onModerate("reject")} disabled={busy}>
              Rejeitar ✕
            </button>
          </>
        )}
        {req.status === "approved" && (
          <>
            <button className="mix-save" onClick={() => onModerate("played")} disabled={busy}>
              Tocado ♪
            </button>
            <button className="mix-close" onClick={() => onModerate("reject")} disabled={busy}>
              Rejeitar ✕
            </button>
          </>
        )}
      </div>
    </li>
  );
}
