"use client";

import { useCallback, useEffect, useRef, useState } from "react";

type Opts = {
  min: number; // min panel width (px) before it snaps to collapsed
  max: number; // max panel width (px)
  initial: number; // starting width (px)
  collapseAt: number; // drag below this width (px) => collapse
  /* Largura mínima que a coluna da ESQUERDA precisa para não ser recortada.
     ⚠️ NÃO É ESTÉTICA. A coluna esquerda é `minmax(0, 1fr)` no grid, ou seja,
     aceita encolher até zero, e o `main.op` tem `overflow-x: hidden` — então,
     passado o ponto em que o conteúdo não cabe, ele é CORTADO em silêncio, sem
     barra de rolagem que denuncie. Medido numa janela de 1000px com o painel em
     585: o player precisava de 468px e tinha 406, perdendo 62px de título,
     decks e transporte. */
  leftMin?: number;
  storageKey?: string; // persist width/collapsed across reloads
};

type Persisted = { width: number; collapsed: boolean };

/* Drag-to-resize width for a right-side panel. Dragging the divider right
   shrinks the panel; below `collapseAt` it collapses. `expand` reopens it. */
export function useResizablePanel(o: Opts) {
  const [width, setWidth] = useState(o.initial);
  /* Teto EFETIVO: o menor entre o máximo configurado e o que sobra da janela
     depois de reservar a esquerda. Numa tela larga vale o `max`; numa estreita,
     é isto que impede o painel de comer o player. */
  const ceiling = useCallback(
    (w: number) => {
      const left = o.leftMin ?? 0;
      if (typeof window === "undefined" || !left) return o.max;
      return Math.max(o.min, Math.min(o.max, window.innerWidth - left, w));
    },
    [o.max, o.min, o.leftMin],
  );
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
        // ⚠️ Re-aplica o teto AO CARREGAR: a largura foi salva noutra janela,
        // possivelmente bem maior. Sem isto, abrir num monitor menor restaura um
        // painel largo demais e o player nasce cortado — que é exatamente como o
        // problema apareceu.
        if (typeof p.width === "number")
          setWidth(ceiling(clamp(p.width, o.min, o.max)));
        if (typeof p.collapsed === "boolean") setCollapsed(p.collapsed);
      }
    } catch {
      /* ignore bad storage */
    }
    loaded.current = true;
  }, [o.storageKey, o.min, o.max, ceiling]);

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
      setWidth(ceiling(clamp(w, o.min, o.max)));
    },
    [o.collapseAt, o.min, o.max, ceiling],
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

  /* Redimensionar a JANELA também aperta a esquerda — e aí ninguém arrastou
     nada. Sem este ajuste, encolher a janela corta o player em silêncio. */
  useEffect(() => {
    if (!o.leftMin) return;
    const onResize = () => setWidth((w) => ceiling(w));
    window.addEventListener("resize", onResize);
    onResize();
    return () => window.removeEventListener("resize", onResize);
  }, [o.leftMin, ceiling]);

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
