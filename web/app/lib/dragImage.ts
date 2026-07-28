/* Ghost de drag = só a CAPA, não a row inteira. HTML5 nativo usa a linha toda
   como imagem fantasma; aqui trocamos por um clone da `.track-cover` da própria
   linha, com efeito (ampliada + sombra + leve rotação) via classe CSS.

   Uso: no `onDragStart`, chamar `dragCover(e)`. Acha a capa dentro de
   `e.currentTarget`, clona, aplica o ghost e agenda a remoção (o browser tira o
   snapshot síncrono, então remover no próximo tick é seguro). No-op se não achar
   capa (placeholder ainda conta — é `.track-cover` também). */
export function dragCover(e: React.DragEvent): void {
  const host = e.currentTarget as HTMLElement;
  const cover = host.querySelector<HTMLElement>(".track-cover");
  if (!cover) return;
  const r = cover.getBoundingClientRect();
  const ghost = cover.cloneNode(true) as HTMLElement;
  ghost.classList.add("cover-drag-ghost");
  // dimensão AMPLIADA cravada no clone (snapshot do setDragImage ignora `transform:
  // scale` em alguns browsers — então crescemos por width/height reais). O resto do
  // efeito (sombra/ring/rotação leve) vem da classe .cover-drag-ghost.
  const SCALE = 2.4;
  const w = r.width * SCALE;
  const h = r.height * SCALE;
  ghost.style.width = `${w}px`;
  ghost.style.height = `${h}px`;
  document.body.appendChild(ghost);
  e.dataTransfer.setDragImage(ghost, w / 2, h / 2);
  // remove após o snapshot (mesmo frame). requestAnimationFrame garante que já
  // foi capturado.
  requestAnimationFrame(() => ghost.remove());
}
