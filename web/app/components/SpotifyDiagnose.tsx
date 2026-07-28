"use client";

import { useState } from "react";
import { spotifyDiagnose, type SpotifyDiagnosis } from "@/lib/api";

/* Botão "Diagnosticar" do painel do Spotify.

   Por que existe: houve um caso em que TODOS os sinais normais diziam "está tudo
   bem" — o SDK conectava, o aparelho registrava, o comando de tocar respondia 204
   e nenhum evento de erro era emitido — e mesmo assim não saía som. Descobrir a
   causa (a conta não estava na lista de usuários de um app em Development Mode)
   levou horas de verificação manual. Esta tela roda a mesma cadeia em segundos.

   Os degraus aparecem na ordem causal: o PRIMEIRO que falha é a causa; os de baixo
   nem chegam a ser executados, então não confundem quem está lendo. */
export function SpotifyDiagnose() {
  const [res, setRes] = useState<SpotifyDiagnosis | null>(null);
  const [busy, setBusy] = useState(false);

  const run = async () => {
    setBusy(true);
    try {
      setRes(await spotifyDiagnose());
    } catch {
      setRes({
        ok: false,
        verdict: "Não foi possível falar com o servidor do AutoDJ.",
        steps: [],
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="spdiag">
      <button className="spdiag-btn" onClick={run} disabled={busy}>
        {busy ? "Verificando…" : "🩺 Diagnosticar"}
      </button>

      {res && (
        <div className={"spdiag-out" + (res.ok ? " ok" : " bad")}>
          <ol className="spdiag-steps">
            {res.steps.map((s) => (
              <li key={s.key} className={s.ok ? "on" : "off"}>
                <span className="spdiag-ico">{s.ok ? "✓" : "✕"}</span>
                <span className="spdiag-txt">
                  <b>{s.label}</b> — {s.detail}
                  {s.hint && <em className="spdiag-hint">{s.hint}</em>}
                </span>
              </li>
            ))}
          </ol>
          <p className="spdiag-verdict">
            {res.ok ? "Tudo certo — o Spotify está pronto para tocar." : res.verdict}
          </p>
        </div>
      )}
    </div>
  );
}
