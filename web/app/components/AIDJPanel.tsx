"use client";

import { useAIDJ } from "@/hooks/useAIDJ";
import { useNowPlaying } from "@/hooks/useNowPlaying";
import { AIDJ_VOICES, type AIDJVoice } from "@/lib/api";

/* AI DJ (locução falada, Story 6.4) — modal aberto pela pill 🎙 AI DJ da barra
   Operacional. Liga/desliga os segues falados e escolhe a voz. A voz sintetizada
   é source=tts: PCM, mixável, então entra no bus local e faz a ponte de uma faixa
   Spotify (DRM) — o SDK do Spotify só abaixa o próprio volume enquanto a locução
   toca localmente; áudio DRM nunca entra no mix (invariante dois-motores). "Gerar
   locução" sintetiza (ou lê do cache por hash) a fala da próxima faixa da fila e a
   enfileira à frente dela (pos 0 = tocar em seguida). Backend sem AIDJService
   (rotas 404) → "TTS não configurado"; 503 na locução = nenhum gateway TTS
   (ElevenLabs/Piper) no servidor. */
export function AIDJPanel({ onClose }: { onClose: () => void }) {
  const {
    config,
    configured,
    loaded,
    busy,
    error,
    lastVoiceover,
    toggle,
    setVoice,
    voiceover,
  } = useAIDJ();
  const { snap, enqueue } = useNowPlaying();

  const next = snap?.queue?.[0]?.playable;
  const nextId = next?.track_id;

  const generate = async () => {
    if (!nextId) return;
    const t = await voiceover(nextId);
    if (t?.id) await enqueue(t.id, 0, false); // toca em seguida, antes da faixa
  };

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">🎙 AI DJ — Locução Falada</header>

        <div className="mix-body aidj">
          <div className="aidj-bar">
            {!configured ? (
              <span className="aidj-conn off">TTS não configurado</span>
            ) : (
              <label className="aidj-switch">
                <input
                  type="checkbox"
                  checked={config.enabled}
                  onChange={(e) => toggle(e.target.checked)}
                  disabled={busy || !loaded}
                />
                <span>
                  {config.enabled
                    ? "Segues falados: LIGADO"
                    : "Segues falados: desligado"}
                </span>
              </label>
            )}
          </div>

          {error && <div className="aidj-warn aidj-err">⚠️ {error}</div>}

          <div className="aidj-warn">
            🔊 A voz é sintetizada em <b>PCM</b> e entra no bus local — é ela que
            faz a ponte de uma faixa Spotify (DRM) sem o áudio protegido entrar no
            mix.
          </div>

          <div className="aidj-row">
            <label className="aidj-label">Voz</label>
            <select
              className="aidj-voice-sel"
              value={config.voice}
              onChange={(e) => setVoice(e.target.value as AIDJVoice)}
              disabled={busy || !configured}
            >
              {AIDJ_VOICES.map((v) => (
                <option key={v.value} value={v.value}>
                  {v.label}
                </option>
              ))}
            </select>
          </div>

          <div className="aidj-row">
            <button
              className="aidj-btn"
              onClick={generate}
              disabled={busy || !configured || !config.enabled || !nextId}
              title={
                !config.enabled
                  ? "Ligue os segues falados primeiro"
                  : !nextId
                    ? "Sem próxima faixa na fila"
                    : "Sintetiza e enfileira a locução da próxima faixa"
              }
            >
              {busy ? "Gerando…" : "Gerar locução da próxima"}
            </button>
            {next && <span className="aidj-next">Próxima: {next.title}</span>}
          </div>

          {lastVoiceover && (
            <div className="aidj-warn aidj-ok">
              ✓ Locução pronta: <b>{lastVoiceover.title}</b> (
              {lastVoiceover.source}, mixável) — enfileirada à frente da faixa.
            </div>
          )}
        </div>

        <footer className="mix-footer">
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
