/* iframe wrapper — shows fallback note when the site refuses framing */
export function WebFrame({ url, framed }: { url: string; framed: boolean }) {
  return (
    <div className="frame-wrap">
      <iframe
        className="web-frame"
        src={url}
        title="Web"
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox"
        referrerPolicy="no-referrer"
      />
      {!framed && (
        <div className="frame-note">
          <div className="fn-ico">🖥️</div>
          <div className="fn-title">Este site bloqueia embutir no navegador</div>
          <a className="fn-open" href={url} target="_blank" rel="noreferrer">
            Abrir em nova aba ↗
          </a>
        </div>
      )}
    </div>
  );
}
