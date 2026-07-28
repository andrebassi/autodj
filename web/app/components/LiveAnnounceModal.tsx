"use client";

import { useEffect, useState } from "react";
import { ANNOUNCE_SCRIPT, ANNOUNCE_TITLE } from "@/lib/announce";

/* teleprompter window — "TEXTO PARA LOCUÇÃO AO VIVO" (A+/A-, script, timer, transport) */
export function LiveAnnounceModal({
  script = ANNOUNCE_SCRIPT,
  onClose,
}: {
  script?: string[];
  onClose: () => void;
}) {
  const [font, setFont] = useState(20);
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    const id = setInterval(() => setElapsed((e) => e + 1), 1000);
    return () => clearInterval(id);
  }, []);

  const mm = String(Math.floor(elapsed / 60)).padStart(2, "0");
  const ss = String(elapsed % 60).padStart(2, "0");

  return (
    <div className="loc-overlay" onClick={onClose}>
      <div className="loc-modal" onClick={(e) => e.stopPropagation()}>
        <header className="loc-titlebar">
          <span className="loc-mic">🎙</span>
          <span className="loc-title">{ANNOUNCE_TITLE}</span>
          <span className="loc-wbtns">
            <button className="loc-wbtn">–</button>
            <button className="loc-wbtn">▢</button>
            <button className="loc-wbtn close" onClick={onClose}>
              ✕
            </button>
          </span>
        </header>

        <div className="loc-toolbar">
          <button className="loc-font" onClick={() => setFont((f) => Math.min(40, f + 2))}>
            A+
          </button>
          <button className="loc-font" onClick={() => setFont((f) => Math.max(12, f - 2))}>
            A-
          </button>
        </div>

        <div className="loc-script" style={{ fontSize: `${font}px` }}>
          {script.map((line, i) => (
            <p key={i} className="loc-line">
              {line}
            </p>
          ))}
        </div>

        <div className="loc-progress">
          <span className="loc-pfill" style={{ width: "100%" }} />
        </div>

        <footer className="loc-footer">
          <div className="loc-elapsed">
            {`00:${mm}:${ss}`} <span className="loc-clock">🕐</span>
          </div>
          <div className="loc-controls">
            <button className="loc-ctl">▾</button>
            <button className="loc-ctl">⏩</button>
            <button className="loc-ctl">⏸</button>
          </div>
        </footer>
      </div>
    </div>
  );
}
