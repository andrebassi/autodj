"use client";

/* draggable vertical splitter between the operator and web panels */
export function PanelDivider({
  onStart,
  onMove,
  onEnd,
  onCollapse,
  active,
}: {
  onStart: () => void;
  onMove: (clientX: number) => void;
  onEnd: () => void;
  onCollapse: () => void;
  active: boolean;
}) {
  return (
    <div
      className={"panel-divider" + (active ? " dragging" : "")}
      role="separator"
      aria-orientation="vertical"
      title="Arraste para redimensionar · dê dois cliques para fechar"
      // Pointer Events + pointer capture: the divider keeps receiving move/up
      // even when the cursor leaves it — the only model WKWebView (Wails)
      // delivers reliably mid-drag. Unifies mouse/touch/pen, so no separate
      // touch handlers. preventDefault() suppresses native text selection.
      onPointerDown={(e) => {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        onStart();
      }}
      onPointerMove={(e) => {
        if (active) onMove(e.clientX);
      }}
      onPointerUp={(e) => {
        e.currentTarget.releasePointerCapture(e.pointerId);
        onEnd();
      }}
      onDoubleClick={onCollapse}
    >
      <span className="panel-divider-grip" />
    </div>
  );
}
