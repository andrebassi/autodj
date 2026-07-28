"use client";

import { type UserDTO } from "@/lib/api";

/* titlebar account actions (Story 7.3): Layout / Limpar. "Limpar" zera a playout
   (limpa a fila + para a faixa atual), começando do zero, com confirmação elegante
   disparada pelo page.tsx. No operator identity text and no user-management entry
   here — user management lives outside the single-operator titlebar. */
export function UserBadge({
  onLayout,
  onClear,
}: {
  user: UserDTO;
  onLayout: () => void;
  onClear: () => void;
}) {
  return (
    <div className="user-badge">
      <button
        className="user-btn"
        onClick={onLayout}
        title="Configurações e aparência do player"
      >
        ⚙ Configurações
      </button>
      <button
        className="user-btn"
        onClick={onClear}
        title="Limpar tudo e começar do zero"
      >
        🧹 Limpar
      </button>
    </div>
  );
}
