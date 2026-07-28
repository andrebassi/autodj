"use client";

import { useEffect, useState } from "react";
import { useMix } from "@/hooks/useMix";
import { type MixConfig } from "@/lib/uiContent";

/* mini-gráfico fade in/out (envelope) — só visual */
function FadeGraph({ inMs, outMs }: { inMs: number; outMs: number }) {
  const span = Math.max(inMs + outMs, 1);
  const inPct = (inMs / span) * 40;
  const outPct = (outMs / span) * 40;
  return (
    <svg className="mix-graph" viewBox="0 0 100 40" preserveAspectRatio="none">
      <polyline
        className="mix-in"
        points={`4,36 ${4 + inPct},4 96,4`}
        fill="none"
      />
      <polyline
        className="mix-out"
        points={`4,4 ${96 - outPct},4 96,36`}
        fill="none"
      />
    </svg>
  );
}

function Group({
  title,
  inMs,
  outMs,
  onIn,
  onOut,
}: {
  title: string;
  inMs: number;
  outMs: number;
  onIn: (v: number) => void;
  onOut: (v: number) => void;
}) {
  return (
    <fieldset className="mix-group">
      <legend>{title}</legend>
      <div className="mix-row">
        <span className="mix-lbl">Fade In</span>
        <input
          className="mix-num"
          type="number"
          value={inMs}
          onChange={(e) => onIn(+e.target.value)}
        />
        <FadeGraph inMs={inMs} outMs={outMs} />
        <input
          className="mix-num"
          type="number"
          value={outMs}
          onChange={(e) => onOut(+e.target.value)}
        />
        <span className="mix-lbl">Fade Out</span>
      </div>
    </fieldset>
  );
}

/* Configuração de Mixagem — fades por tipo, sobreposição, falar, normalização */
export function MixConfigModal({
  mix,
  onClose,
}: {
  mix?: MixConfig;
  onClose: () => void;
}) {
  const { mix: loaded, saving, error, save } = useMix(mix);
  const [c, setC] = useState<MixConfig>(mix ?? loaded);
  const set = (patch: Partial<MixConfig>) => setC((p) => ({ ...p, ...patch }));

  // adopt the persisted values once the fetch resolves.
  useEffect(() => {
    setC(loaded);
  }, [loaded]);

  const onSave = async () => {
    if (await save(c)) onClose();
  };

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">
          AutoDJ — Configuração de Mixagem
        </header>

        <div className="mix-body">
          <Group
            title="Mixagem de Músicas"
            inMs={c.musicaIn}
            outMs={c.musicaOut}
            onIn={(v) => set({ musicaIn: v })}
            onOut={(v) => set({ musicaOut: v })}
          />
          <Group
            title="Mixagem de Vinhetas"
            inMs={c.vinhetaIn}
            outMs={c.vinhetaOut}
            onIn={(v) => set({ vinhetaIn: v })}
            onOut={(v) => set({ vinhetaOut: v })}
          />
          <Group
            title="Mixagem de Comerciais"
            inMs={c.comercialIn}
            outMs={c.comercialOut}
            onIn={(v) => set({ comercialIn: v })}
            onOut={(v) => set({ comercialOut: v })}
          />

          <fieldset className="mix-group">
            <legend>Nível do Volume na Sobreposição</legend>
            <div className="mix-inline">
              Baixar o volume da mídia em execução na sobreposição para
              <input
                className="mix-num"
                type="number"
                value={c.overlapVol}
                onChange={(e) => set({ overlapVol: +e.target.value })}
              />
              %
            </div>
          </fieldset>

          <fieldset className="mix-group">
            <legend>Nível do Volume para Falar</legend>
            <div className="mix-inline">
              Baixar o volume da mídia em execução na opção Falar para
              <input
                className="mix-num"
                type="number"
                value={c.talkVol}
                onChange={(e) => set({ talkVol: +e.target.value })}
              />
              %
            </div>
          </fieldset>

          <div className="mix-checks">
            <label>
              <input
                type="checkbox"
                checked={c.trimMusicas}
                onChange={(e) => set({ trimMusicas: e.target.checked })}
              />
              Tirar Silêncio das Músicas
            </label>
            <label>
              <input
                type="checkbox"
                checked={c.trimVinhetas}
                onChange={(e) => set({ trimVinhetas: e.target.checked })}
              />
              Tirar Silêncio das Vinhetas
            </label>
            <label>
              <input
                type="checkbox"
                checked={c.normalize}
                onChange={(e) => set({ normalize: e.target.checked })}
              />
              Normalizar Volume das Mídias
              <input
                className="mix-num sm"
                type="number"
                value={c.normalizeVol}
                onChange={(e) => set({ normalizeVol: +e.target.value })}
              />
              %
            </label>
          </div>
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-save" onClick={onSave} disabled={saving}>
            {saving ? "Salvando…" : "Salvar ✔"}
          </button>
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
