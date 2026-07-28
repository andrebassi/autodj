"use client";

import { useLineIn } from "@/hooks/useLineIn";
import { type LineInInput } from "@/lib/api";

/* formata um nível [0,1] como percentual inteiro. */
function pct(v: number): string {
  return Math.round(v * 100) + "%";
}

/* Line-in — captura ao vivo (microfone/auxiliar/placa). É a fonte PCM *ao vivo*
   legítima: sempre mixável, nunca DRM — o oposto do Spotify. Armar roteia a
   entrada pro bus local abaixando (duck) a música; ganho/duck ajustam a mistura.
   Não fere o invariante dois-motores (Spotify segue não-mixável). */
export function LineInPanel({ onClose }: { onClose: () => void }) {
  const { inputs, configured, loaded, busy, error, arm, disarm, setGain, setDuck } =
    useLineIn();

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Line-in (Captura ao Vivo)</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}

          {loaded && !configured && (
            <fieldset className="mix-group">
              <legend>Line-in não configurado</legend>
              <p className="mix-lbl">
                O servidor não expôs o serviço de captura. Configure entradas
                (AUTODJ_LINEIN) e reinicie para armar microfone/auxiliar.
              </p>
            </fieldset>
          )}

          {loaded && configured && inputs.length === 0 && (
            <p className="mix-lbl">Nenhuma entrada de captura.</p>
          )}

          {loaded &&
            configured &&
            inputs.map((in_) => (
              <LineInRow
                key={in_.id}
                input={in_}
                busy={busy === in_.id}
                onArm={() => arm(in_.id)}
                onDisarm={() => disarm(in_.id)}
                onGain={(v) => setGain(in_.id, v)}
                onDuck={(v) => setDuck(in_.id, v)}
              />
            ))}
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}

/* uma entrada: botão armar/desarmar + sliders de ganho e duck. */
function LineInRow({
  input,
  busy,
  onArm,
  onDisarm,
  onGain,
  onDuck,
}: {
  input: LineInInput;
  busy: boolean;
  onArm: () => void;
  onDisarm: () => void;
  onGain: (v: number) => void;
  onDuck: (v: number) => void;
}) {
  return (
    <fieldset className="mix-group">
      <legend>
        {input.label}{" "}
        <span className="mono">{input.armed ? "● NO AR" : "○ desarmado"}</span>
      </legend>
      <div className="mix-row">
        <button
          className="mix-save"
          onClick={input.armed ? onDisarm : onArm}
          disabled={busy}
        >
          {input.armed ? "Desarmar ⏹" : "Armar ▶"}
        </button>
      </div>
      <div className="mix-row">
        <span className="mix-lbl">Ganho {pct(input.gain)}</span>
        <input
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={input.gain}
          disabled={busy}
          onChange={(e) => onGain(+e.target.value)}
        />
      </div>
      <div className="mix-row">
        <span className="mix-lbl">Duck da música {pct(input.duck)}</span>
        <input
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={input.duck}
          disabled={busy}
          onChange={(e) => onDuck(+e.target.value)}
        />
      </div>
    </fieldset>
  );
}
