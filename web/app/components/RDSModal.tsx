"use client";

import { useState } from "react";
import { useRDS } from "@/hooks/useRDS";
import { type RDSConfig } from "@/lib/api";

const PTY_NAMES: Record<number, string> = {
  0: "Nenhum",
  1: "Notícias",
  10: "Pop",
  11: "Rock",
  15: "Outra Música",
  16: "Tempo",
  20: "Religião",
  24: "Jazz",
  25: "Country",
};

/* mostra PI em hex 4-dígitos como o padrão CENELEC exibe. */
function hex4(pi: number): string {
  return "0x" + pi.toString(16).toUpperCase().padStart(4, "0");
}

/* RDS — identidade (PI/PS/PTY) + RadioText ao vivo. RDS carrega só metadados
   (texto), nunca áudio; logo não toca no invariante dois-motores. Refresh empurra
   o now-playing atual pro encoder (503 se AUTODJ_RDS_FILE não estiver setado). */
export function RDSModal({ onClose }: { onClose: () => void }) {
  const { config, configured, loaded, busy, error, state, save, refresh } =
    useRDS();
  const [c, setC] = useState<RDSConfig | null>(null);
  const [title, setTitle] = useState("");
  const [artist, setArtist] = useState("");

  // adota a config persistida na primeira renderização com dados.
  const cur = c ?? config;
  const set = (patch: Partial<RDSConfig>) => setC({ ...cur, ...patch });

  const onSave = () => save(cur);
  const onRefresh = () => refresh(title.trim(), artist.trim());

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — RDS (RadioText)</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}

          {loaded && !configured && (
            <fieldset className="mix-group">
              <legend>RDS não configurado</legend>
              <p className="mix-lbl">
                O servidor não expôs o serviço RDS. Wire um encoder
                (AUTODJ_RDS_FILE) e reinicie para editar identidade e enviar
                RadioText.
              </p>
            </fieldset>
          )}

          {loaded && configured && (
            <>
              <fieldset className="mix-group">
                <legend>Identidade da Estação</legend>
                <div className="mix-checks">
                  <label>
                    <input
                      type="checkbox"
                      checked={cur.enabled}
                      onChange={(e) => set({ enabled: e.target.checked })}
                    />
                    RDS Habilitado
                  </label>
                </div>
                <div className="mix-row">
                  <span className="mix-lbl">PS (≤8)</span>
                  <input
                    className="mix-num"
                    type="text"
                    maxLength={8}
                    value={cur.ps}
                    onChange={(e) => set({ ps: e.target.value })}
                  />
                  <span className="mix-lbl">PTY</span>
                  <input
                    className="mix-num"
                    type="number"
                    min={0}
                    max={31}
                    value={cur.pty}
                    onChange={(e) => set({ pty: +e.target.value })}
                  />
                  <span className="mix-lbl">{PTY_NAMES[cur.pty] ?? "—"}</span>
                </div>
                <div className="mix-inline">
                  PI
                  <input
                    className="mix-num"
                    type="number"
                    min={1}
                    max={65535}
                    value={cur.pi}
                    onChange={(e) => set({ pi: +e.target.value })}
                  />
                  {hex4(cur.pi)}
                </div>
              </fieldset>

              <fieldset className="mix-group">
                <legend>RadioText ao Vivo</legend>
                <div className="mix-row">
                  <span className="mix-lbl">Título</span>
                  <input
                    className="mix-num"
                    type="text"
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                  />
                  <span className="mix-lbl">Intérprete</span>
                  <input
                    className="mix-num"
                    type="text"
                    value={artist}
                    onChange={(e) => setArtist(e.target.value)}
                  />
                  <button
                    className="mix-save"
                    onClick={onRefresh}
                    disabled={busy || !cur.enabled}
                  >
                    Enviar ⟳
                  </button>
                </div>
                <div className="mix-inline">
                  <span className="mix-lbl">Atual:</span>
                  <span className="mono">{state?.rt || "—"}</span>
                </div>
                {state && (
                  <div className="mix-inline">
                    <span className="mix-lbl">
                      PS {state.ps} · {hex4(state.pi)} · PTY {state.pty}
                    </span>
                  </div>
                )}
              </fieldset>
            </>
          )}
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          {loaded && configured && (
            <button className="mix-save" onClick={onSave} disabled={busy}>
              {busy ? "Salvando…" : "Salvar ✔"}
            </button>
          )}
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
