"use client";

import { useEffect } from "react";
import { useClockPreview } from "@/hooks/useClockPreview";

/* Prever hora (Story 4.3): renders the read-only playlist a clock's rotation
   rules produce from the current catalog. Runs the generator on mount for the
   given clock and lists the resulting tracks in order (nothing is played). */
export function ClockPreview({
  clockId,
  clockName,
  onClose,
}: {
  clockId: string;
  clockName: string;
  onClose: () => void;
}) {
  const { tracks, loading, error, run } = useClockPreview();

  useEffect(() => {
    run(clockId);
  }, [clockId, run]);

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal preview-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">
          Prever hora — {clockName || clockId}
        </header>

        <div className="mix-body preview-body">
          {loading ? (
            <p className="clocks-empty">Gerando…</p>
          ) : error ? (
            <p className="mix-err">{error}</p>
          ) : (
            <ol className="preview-list">
              {tracks.map((t, i) => (
                <li key={t.id + "-" + i} className="preview-item">
                  <span className="preview-idx">{i + 1}</span>
                  <span className="preview-title">{t.title}</span>
                  <span className="preview-artist">{t.artist}</span>
                  <span className="preview-cat">{t.category}</span>
                  {t.bpm > 0 && (
                    <span className="preview-bpm">{Math.round(t.bpm)} BPM</span>
                  )}
                </li>
              ))}
            </ol>
          )}
        </div>

        <footer className="mix-footer">
          <span className="mix-lbl">
            {tracks.length} faixa{tracks.length === 1 ? "" : "s"}
          </span>
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
