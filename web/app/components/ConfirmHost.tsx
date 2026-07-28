"use client";

import { useSyncExternalStore } from "react";
import {
  subscribeConfirm,
  getConfirm,
  getServerConfirm,
  resolveConfirm,
} from "@/lib/confirm";

/* Diálogo de confirmação Ant. Montado uma vez em page.tsx; visível só quando
   há pedido pendente (via confirmDialog()). z-index acima dos demais modais. */

export function ConfirmHost() {
  const req = useSyncExternalStore(
    subscribeConfirm,
    getConfirm,
    getServerConfirm,
  );
  if (!req) return null;
  return (
    <div className="cfm-overlay" onClick={() => resolveConfirm(false)}>
      <div
        className={`cfm-modal${req.danger ? " danger" : ""}`}
        role="alertdialog"
        aria-modal="true"
        aria-label={req.title}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="cfm-body">
          <span className={`cfm-ico${req.danger ? " danger" : ""}`} aria-hidden>
            {req.danger ? "⚠" : "?"}
          </span>
          <div className="cfm-text">
            <p className="cfm-title">{req.title}</p>
            {req.body && <p className="cfm-desc">{req.body}</p>}
            {req.bullets && req.bullets.length > 0 && (
              <ul className="cfm-list">
                {req.bullets.map((b, i) => (
                  <li
                    key={i}
                    style={{ animationDelay: `${0.09 + i * 0.05}s` }}
                  >
                    {b}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
        <div className="cfm-footer">
          <button className="cfm-cancel" onClick={() => resolveConfirm(false)}>
            {req.cancelText ?? "Cancelar"}
          </button>
          <button
            className={`cfm-ok${req.danger ? " danger" : ""}`}
            onClick={() => resolveConfirm(true)}
            autoFocus
          >
            {req.okText ?? "OK"}
          </button>
        </div>
      </div>
    </div>
  );
}
