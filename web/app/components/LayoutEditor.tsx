"use client";

import { useEffect, useState } from "react";
import { type ProfileDTO } from "@/lib/api";
import { getMode, setMode as persistMode, type PlayerMode } from "@/lib/mode";
import { fadeMs, setFadeMs } from "@/lib/fade";
import { initSession, sessionId } from "@/lib/session";
import { useMix } from "@/hooks/useMix";
import { targetLabel } from "@/lib/gain";
import type { MixConfig } from "@/lib/uiContent";

const THEMES: { v: string; label: string }[] = [
  { v: "claro", label: "Claro" },
  { v: "escuro", label: "Escuro" },
  { v: "auto", label: "Automático (sistema)" },
];
const MODES: { v: PlayerMode; label: string; hint: string }[] = [
  { v: "radio", label: "Rádio", hint: "estação completa (grade, eventos, RDS, ECAD…)" },
  { v: "tocador", label: "Tocador", hint: "só o tocador de música (esconde funções de rádio)" },
];
const DENSITIES: { v: string; label: string }[] = [
  { v: "confortavel", label: "Confortável" },
  { v: "compacto", label: "Compacto" },
];

/* LayoutEditor (Story 7.4): per-operator player appearance. Só aparência agora
   (tema + densidade) — a operação (modo toque / monitores) e o reordenar/ocultar
   painéis saíram a pedido. Save persiste via PUT /profile/me (owner do token) e o
   shell reaplica o layout; touch/screens/panels do profile passam intactos.
   Fecha clicando fora (overlay). Reusa o styling mix-*. Um componente por arquivo. */
export function LayoutEditor({
  profile,
  busy,
  error,
  onSave,
  onClose,
}: {
  profile: ProfileDTO;
  busy: boolean;
  error: string | null;
  onSave: (patch: {
    theme: string;
    density: string;
    touch: boolean;
    screens: number;
    panels: ProfileDTO["panels"];
  }) => Promise<void>;
  onClose: () => void;
}) {
  const [theme, setTheme] = useState(profile.theme);
  const [density, setDensity] = useState(profile.density);
  // modo (rádio/tocador) é client-side (localStorage) — aplica na hora do Salvar.
  const [mode, setModeLocal] = useState<PlayerMode>(getMode());
  // fade nas transições do Tocador (duração ms, 0=corte seco) — client-side.
  const [fade, setFadeLocal] = useState<number>(fadeMs());
  // fingerprint da sessão do browser — chave da estação isolada (SQLite próprio).
  const [sid, setSid] = useState(sessionId());
  useEffect(() => {
    initSession().then(setSid);
  }, []);

  /* Áudio das faixas locais. A configuração é a MESMA do modal de Mixagem (mesma
     rota `/mix`, mesmo hook) — o que muda é o alcance: o ⚙ está sempre visível,
     enquanto o modal de Mixagem é gated no modo Rádio e o padrão do app é
     Tocador. Ou seja, sem este bloco esses interruptores eram inalcançáveis. */
  const mixApi = useMix();
  const [audio, setAudio] = useState<MixConfig | null>(null);
  useEffect(() => {
    // só semeia depois que o GET resolve: começar pelo default e salvar por cima
    // gravaria a configuração de fábrica sobre a escolha do operador.
    if (mixApi.loaded) setAudio(mixApi.mix);
  }, [mixApi.loaded, mixApi.mix]);
  const patchAudio = (p: Partial<MixConfig>) =>
    setAudio((a) => (a ? { ...a, ...p } : a));

  const save = async () => {
    try {
      // aparência muda; touch/screens/panels do profile seguem inalterados.
      await onSave({
        theme,
        density,
        touch: profile.touch,
        screens: profile.screens || 1,
        panels: profile.panels,
      });
      persistMode(mode); // esconde/mostra as funções de rádio conforme o modo
      setFadeMs(fade); // duração do fade nas transições Spotify↔YouTube
      // áudio das faixas locais → PUT /mix (só se o GET já tinha resolvido).
      if (audio) await mixApi.save(audio);
      onClose();
    } catch {
      // error surfaced by the hook; keep the modal open so the user can retry.
    }
  };

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div
        className="mix-modal"
        style={{ maxWidth: 420 }}
        onClick={(e) => e.stopPropagation()}
      >
        <header className="mix-titlebar">AutoDJ — Modo do Player</header>

        <div className="mix-body">
          <fieldset className="mix-group">
            <legend>Aparência</legend>
            <div className="mix-row">
              <span className="mix-lbl">Tema</span>
              <select
                className="mix-select"
                value={theme}
                onChange={(e) => setTheme(e.target.value)}
              >
                {THEMES.map((t) => (
                  <option key={t.v} value={t.v}>
                    {t.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="mix-row">
              <span className="mix-lbl">Modo</span>
              <select
                className="mix-select"
                value={mode}
                onChange={(e) => setModeLocal(e.target.value as PlayerMode)}
                title={MODES.find((m) => m.v === mode)?.hint}
              >
                {MODES.map((m) => (
                  <option key={m.v} value={m.v}>
                    {m.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="mix-row">
              <span className="mix-lbl">Densidade</span>
              <select
                className="mix-select"
                value={density}
                onChange={(e) => setDensity(e.target.value)}
              >
                {DENSITIES.map((d) => (
                  <option key={d.v} value={d.v}>
                    {d.label}
                  </option>
                ))}
              </select>
            </div>
          </fieldset>

          <fieldset className="mix-group">
            <legend>Modo Tocador</legend>
            <div className="mix-row">
              <span className="mix-lbl">Fade nas transições</span>
              <select
                className="mix-select"
                value={String(fade)}
                onChange={(e) => setFadeLocal(Number(e.target.value))}
                title="Só na troca de motor Spotify↔YouTube: fade-out no que toca + fade-in na nova, na duração escolhida. Mesmo motor (spotify→spotify, youtube→youtube) e STOP = corte seco."
              >
                <option value="3000">Ligado (fade de 3s)</option>
                <option value="5000">Ligado (fade de 5s)</option>
                <option value="7000">Ligado (fade de 7s)</option>
                <option value="0">Desligado (corte seco)</option>
              </select>
            </div>
          </fieldset>

          {/* Só faixas LOCAIS: Spotify e YouTube tocam fora daqui (sem PCM do lado
              do browser), então não há o que medir nem o que cortar neles. */}
          <fieldset className="mix-group">
            <legend>Áudio das faixas locais</legend>
            {!audio ? (
              <div className="mix-row">
                <span className="mix-lbl">carregando…</span>
              </div>
            ) : (
              <>
                <label className="mix-row">
                  <span className="mix-lbl">Cortar silêncio das músicas</span>
                  <input
                    type="checkbox"
                    className="mix-check"
                    checked={audio.trimMusicas}
                    onChange={(e) => patchAudio({ trimMusicas: e.target.checked })}
                    title="Começa a música onde o áudio começa de fato e encerra onde ele acaba — sem o vazio das pontas."
                  />
                </label>
                <label className="mix-row">
                  <span className="mix-lbl">Cortar silêncio das vinhetas</span>
                  <input
                    type="checkbox"
                    className="mix-check"
                    checked={audio.trimVinhetas}
                    onChange={(e) => patchAudio({ trimVinhetas: e.target.checked })}
                    title="Mesmo corte, para as vinhetas. Desligado por padrão: cortar o silêncio de uma vinheta muda o tempo dela."
                  />
                </label>
                <label className="mix-row">
                  <span className="mix-lbl">Volume igual entre as faixas</span>
                  <input
                    type="checkbox"
                    className="mix-check"
                    checked={audio.normalize}
                    onChange={(e) => patchAudio({ normalize: e.target.checked })}
                    title="Música masterizada alta e música antiga saem no mesmo volume percebido."
                  />
                </label>
                {audio.normalize && (
                  <div className="mix-row">
                    {/* O número sozinho não diz nada: mostrar o LUFS resultante ao
                        lado é o que impede o knob de mentir sobre o próprio efeito. */}
                    <span className="mix-lbl">Nível alvo</span>
                    <input
                      type="range"
                      className="mix-range"
                      min={0}
                      max={100}
                      value={audio.normalizeVol}
                      onChange={(e) =>
                        patchAudio({ normalizeVol: Number(e.target.value) })
                      }
                    />
                    <span className="mix-hint">{targetLabel(audio.normalizeVol)}</span>
                  </div>
                )}
                <label className="mix-row">
                  <span className="mix-lbl">Detectar entrada do vocal</span>
                  <input
                    type="checkbox"
                    className="mix-check"
                    checked={audio.detectVocal}
                    onChange={(e) => patchAudio({ detectVocal: e.target.checked })}
                    title="Marca onde a voz entra, para encaixar vinheta/locução e usar o LOOP na base. Faixa duvidosa não recebe marca."
                  />
                </label>
                <label className="mix-row">
                  <span className="mix-lbl">Processar SEM-VOZ automaticamente (IA)</span>
                  <input
                    type="checkbox"
                    className="mix-check"
                    checked={audio.autoInstrumental}
                    onChange={(e) =>
                      patchAudio({ autoInstrumental: e.target.checked })
                    }
                    title="Toda música local nova entra na fila da separação por IA assim que é adicionada, sem você pedir faixa a faixa."
                  />
                </label>
                {audio.autoInstrumental && (
                  <p className="mix-note mix-warn">
                    A separação leva cerca de 1,4× a duração de cada música, uma de
                    cada vez. Um envio de 80 faixas ocupa a máquina por várias horas
                    em segundo plano — e só funciona onde a ferramenta está
                    instalada (não vale no servidor).
                  </p>
                )}
                <p className="mix-note">
                  Vale a partir da próxima faixa — a que está no ar não é
                  reposicionada no meio.
                </p>
              </>
            )}
          </fieldset>

          <fieldset className="mix-group">
            <legend>Sessão do browser</legend>
            <div className="mix-row">
              <span className="mix-lbl">ID</span>
              <code
                className="mix-sid"
                title="Fingerprint deste browser (FingerprintJS). Identifica sua estação isolada — biblioteca, fila e configs próprios (SQLite ~/.autodj/sessions/<id>.db)."
                onClick={() => {
                  if (sid) navigator.clipboard?.writeText(sid).catch(() => {});
                }}
              >
                {sid || "resolvendo…"}
              </code>
            </div>
          </fieldset>
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-save" onClick={save} disabled={busy}>
            {busy ? "Salvando…" : "Salvar"}
          </button>
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
