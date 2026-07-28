import { CHANGELOG } from "@/lib/changelog";
import { APP_VERSION } from "@/lib/version";

/* Popup do changelog — aberto ao clicar na versão do titlebar. Lista as versões
   (mais recente primeiro) com destaques, e no rodapé o link do autor. Segue a
   convenção .mix-overlay/.mix-modal. */
export function ChangelogModal({ onClose }: { onClose: () => void }) {
  return (
    <div className="mix-overlay" onClick={onClose}>
      <div
        className="mix-modal cl-modal"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mix-titlebar">
          <span>
            <span className="cl-mark">▲</span> AutoDJ — Novidades
          </span>
          <span className="cl-cur">{APP_VERSION}</span>
        </div>

        <div className="cl-body">
          <ol className="cl-list">
            {CHANGELOG.map((e, i) => (
              <li key={e.version} className={"cl-item" + (i === 0 ? " on" : "")}>
                <div className="cl-dot" aria-hidden />
                <div className="cl-content">
                  <div className="cl-head">
                    <span className="cl-ver">v{e.version}</span>
                    <span className="cl-title">{e.title}</span>
                    <span className="cl-date">{e.date}</span>
                  </div>
                  <ul className="cl-hl">
                    {e.highlights.map((h, j) => (
                      <li key={j}>{h}</li>
                    ))}
                  </ul>
                </div>
              </li>
            ))}
          </ol>
        </div>

        <div className="mix-footer cl-footer">
          <a
            className="cl-author"
            href="https://andrebassi.com.br"
            target="_blank"
            rel="noopener noreferrer"
          >
            andrebassi.com.br ↗
          </a>
          <button className="mix-close" onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  );
}
