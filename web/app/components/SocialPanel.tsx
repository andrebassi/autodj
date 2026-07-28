"use client";

import { useState } from "react";
import { useSocial } from "@/hooks/useSocial";
import type { SocPlatformDTO, SocReceiptDTO } from "@/lib/api";

/* SocialPanel — social integrations (Story 8.5). Two one-click actions: announce
   the current now-playing track across selected platforms, or post a free-text
   note to one platform. Platforms come from the API (no hardcoded list); the home
   build publishes through a credential-free stub gateway, so the receipts are
   synthetic (stub:// URLs) — swapping a real gateway is just an adapter change.
   This is metadata, never audio. */
export function SocialPanel({ onClose }: { onClose: () => void }) {
  const { platforms, loaded, busy, available, error, receipts, announce, post } = useSocial();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [customPlatform, setCustomPlatform] = useState("");
  const [text, setText] = useState("");

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const selectedKeys = platforms.map((p) => p.key).filter((k) => selected.has(k));
  const activePlatform = customPlatform || (platforms[0]?.key ?? "");
  const maxLen = platforms.find((p) => p.key === activePlatform)?.max_len ?? 0;

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Redes sociais</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}
          {loaded && !available && (
            <p className="mix-lbl">Redes sociais indisponíveis (endpoint não configurado).</p>
          )}
          {loaded && available && (
            <>
              <fieldset className="mix-group">
                <legend>Anunciar tocando agora</legend>
                <div className="soc-platforms">
                  {platforms.map((p: SocPlatformDTO) => (
                    <label className="soc-chk" key={p.key}>
                      <input
                        type="checkbox"
                        checked={selected.has(p.key)}
                        onChange={() => toggle(p.key)}
                      />
                      {p.label}
                    </label>
                  ))}
                </div>
                <div className="mix-footer">
                  <button
                    className="mix-save"
                    onClick={() => void announce(selectedKeys)}
                    disabled={busy || selectedKeys.length === 0}
                  >
                    Anunciar tocando agora
                  </button>
                </div>
              </fieldset>

              <fieldset className="mix-group">
                <legend>Recado livre</legend>
                <div className="mix-row">
                  <label className="mix-lbl">Plataforma</label>
                  <select
                    className="soc-select"
                    value={activePlatform}
                    onChange={(e) => setCustomPlatform(e.target.value)}
                  >
                    {platforms.map((p) => (
                      <option value={p.key} key={p.key}>
                        {p.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="mix-row">
                  <textarea
                    className="soc-text"
                    value={text}
                    maxLength={maxLen > 0 ? maxLen : undefined}
                    placeholder="Digite o recado…"
                    onChange={(e) => setText(e.target.value)}
                  />
                </div>
                <div className="mix-footer">
                  {maxLen > 0 && (
                    <span className="mix-lbl mono">
                      {text.length}/{maxLen}
                    </span>
                  )}
                  <button
                    className="mix-save"
                    onClick={() => {
                      void post(activePlatform, text);
                      setText("");
                    }}
                    disabled={busy || text.trim() === "" || activePlatform === ""}
                  >
                    Publicar
                  </button>
                </div>
              </fieldset>

              <fieldset className="mix-group">
                <legend>Publicados</legend>
                {receipts.length === 0 ? (
                  <p className="mix-lbl">Nada publicado ainda.</p>
                ) : (
                  <ul className="soc-list">
                    {receipts.map((rc: SocReceiptDTO, i) => (
                      <li className="soc-receipt" key={rc.external_id + "-" + i}>
                        <span className={"pub-badge pub-src-" + rc.platform}>{rc.platform}</span>
                        <span className="soc-url mono">{rc.url}</span>
                        <span className="pub-time mono">{fmtTime(rc.published_at)}</span>
                      </li>
                    ))}
                  </ul>
                )}
              </fieldset>
            </>
          )}
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-close" onClick={onClose}>
            Fechar
          </button>
        </footer>
      </div>
    </div>
  );
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "--:--";
  return d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
}
