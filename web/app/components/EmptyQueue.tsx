"use client";

/* Estado-vazio da fila (nada engatilhado além dos decks). Só as notinhas musicais
   flutuando (sem disco) + dica p/ arrastar mídia. Elegante e leve. */
const NOTES = ["♪", "♫", "♩", "♬", "♪"];

export function EmptyQueue() {
  return (
    <li className="empty-queue" aria-hidden="true">
      <div className="eq-notes">
        {NOTES.map((n, i) => (
          <span key={i} className={"eq-note eq-note-" + (i + 1)}>
            {n}
          </span>
        ))}
      </div>
      <div className="eq-text">
        <strong className="eq-title">Fila vazia</strong>
        <span className="eq-sub">Arraste uma música pra cá — ou toque play e deixe rolar 🎧</span>
      </div>
    </li>
  );
}
