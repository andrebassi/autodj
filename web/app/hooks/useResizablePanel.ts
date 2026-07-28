"use client";

import { useCallback, useEffect, useRef, useState } from "react";

type Opts = {
  min: number; // min panel width (px) before it snaps to collapsed
  max: number; // max panel width (px)
  initial: number; // starting width (px)
  collapseAt: number; // drag below this width (px) => collapse
  storageKey?: string; // persist width/collapsed across reloads
};

type Persisted = { width: number; collapsed: boolean };

/* Drag-to-resize width for a right-side panel. Dragging the divider right
   shrinks the panel; below `collapseAt` it collapses. `expand` reopens it. */
export function useResizablePanel(o: Opts) {
  const [width, setWidth] = useState(o.initial);
  const [collapsed, setCollapsed] = useState(false);
  const [dragging, setDragging] = useState(false);
  const loaded = useRef(false);

  // hydrate from storage once (client-only, avoids SSR mismatch)
  useEffect(() => {
    if (!o.storageKey) return;
    try {
      const raw = localStorage.getItem(o.storageKey);
      if (raw) {
        const p = JSON.parse(raw) as Persisted;
        if (typeof p.width === "number") setWidth(clamp(p.width, o.min, o.max));
        if (typeof p.collapsed === "boolean") setCollapsed(p.collapsed);
      }
    } catch {
      /* ignore bad storage */
    }
    loaded.current = true;
  }, [o.storageKey, o.min, o.max]);

  useEffect(() => {
    if (!o.storageKey || !loaded.current) return;
    try {
      localStorage.setItem(o.storageKey, JSON.stringify({ width, collapsed }));
    } catch {
      /* ignore */
    }
  }, [width, collapsed, o.storageKey]);

  const onMove = useCallback(
    (clientX: number) => {
      // panel hugs the right edge; its width is the gap to the viewport edge
      const w = window.innerWidth - clientX;
      if (w < o.collapseAt) {
        setCollapsed(true);
        setDragging(false);
        return;
      }
      setWidth(clamp(w, o.min, o.max));
    },
    [o.collapseAt, o.min, o.max],
  );

  const startDrag = useCallback(() => {
    setCollapsed(false);
    setDragging(true);
  }, []);

  const endDrag = useCallback(() => setDragging(false), []);

  // Only a body cursor hint while dragging — no global move/up listeners.
  // The divider owns the pointer via setPointerCapture (see PanelDivider), which
  // is the one path WKWebView (Wails desktop) delivers reliably: global
  // window `mousemove` with the button held is dropped there mid-drag, so the
  // browser resized fine but the native window did not.
  useEffect(() => {
    if (!dragging) return;
    document.body.style.cursor = "col-resize";
    return () => {
      document.body.style.cursor = "";
    };
  }, [dragging]);

  const collapse = useCallback(() => setCollapsed(true), []);
  const expand = useCallback(() => setCollapsed(false), []);

  return {
    width,
    collapsed,
    dragging,
    startDrag,
    onMove,
    endDrag,
    collapse,
    expand,
  };
}

function clamp(v: number, lo: number, hi: number) {
  return Math.max(lo, Math.min(hi, v));
}
