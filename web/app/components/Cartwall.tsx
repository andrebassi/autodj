"use client";

import { useMemo, useState } from "react";
import { useCartwall } from "@/hooks/useCartwall";
import { useLibrary } from "@/hooks/useLibrary";
import { CartButton, type TrackOption } from "@/components/CartButton";
import { CartwallToolbar } from "@/components/CartwallToolbar";

/* cartwall Instantâneas — parede de botões reais (Story 5.4): board persistido
   por usuário (via API), disparo pelo grafo Web Audio, hotkeys por página e
   modos de disparo. Só orquestra: board/fire/hotkeys moram em useCartwall; a
   lista de faixas mixáveis (picker de edição) vem de useLibrary. */
export function Cartwall() {
  const cw = useCartwall();
  const { tracks } = useLibrary();
  const [editing, setEditing] = useState(false);

  // só faixas mixáveis (local + PCM) podem ser atribuídas — 6ª guarda do invariante
  const trackOptions: TrackOption[] = useMemo(
    () =>
      tracks
        .filter((t) => t.id && t.source === "local" && t.pcm)
        .map((t) => ({ id: t.id as string, label: `${t.artist} — ${t.title}` })),
    [tracks],
  );

  const pageButtons = cw.buttonsOn(cw.page);

  return (
    <div className="cw">
      {/* barra de páginas só quando há +de uma: com uma só página o chip
          "Instantâneas" do topo já indica onde estamos (evita poluição visual) */}
      {cw.pages.length > 1 && (
        <div className="cw-tabs">
          {cw.pages.map((p) => (
            <button
              key={p}
              className={"cw-tab" + (cw.page === p ? " on" : "")}
              onClick={() => cw.setPage(p)}
            >
              ⚡ Instantâneas {p}
            </button>
          ))}
        </div>
      )}

      {cw.error && <div className="cw-err">{cw.error}</div>}

      <div className="cw-grid">
        {pageButtons.map((b) => (
          <CartButton
            key={b.id}
            btn={b}
            editing={editing}
            trackOptions={trackOptions}
            onFire={cw.fire}
            onAssign={cw.assign}
            onClear={cw.clear}
          />
        ))}
        {pageButtons.length === 0 && (
          <div className="cw-empty">Sem botões nesta página.</div>
        )}
      </div>

      <CartwallToolbar
        mode={cw.mode}
        onMode={cw.setMode}
        loop={cw.loop}
        onToggleLoop={cw.toggleLoop}
        onStop={cw.stopAll}
        onLouder={cw.louder}
        onSofter={cw.softer}
        editing={editing}
        onToggleEdit={() => setEditing((v) => !v)}
      />
    </div>
  );
}
