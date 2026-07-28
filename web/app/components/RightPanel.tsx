"use client";

import { OperacionalScreen } from "@/components/OperacionalScreen";
import { WebPanel } from "@/components/WebPanel";
import { useMode } from "@/lib/mode";

export type PanelMode = "operacional" | "internet";

/* painel direito multi-modo — alterna Operacional <-> Internet.
   Chrome fixo: switch de modo + ✕ fechar. Player fica fixo à esquerda.
   Internet é função de estação: só aparece no modo Rádio. */
export function RightPanel({
  mode,
  onMode,
  onClose,
  onEnqueue,
  onEnqueueSpotify,
  onEnqueueYouTube,
  onEnqueueYouTubeLinks,
}: {
  mode: PanelMode;
  onMode: (m: PanelMode) => void;
  onClose: () => void;
  onEnqueue?: (trackId: string, pos: number, playNow: boolean) => void;
  onEnqueueSpotify?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onEnqueueYouTube?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  // enfileira um LOTE de links colados (sub-aba "Colar links" do YouTube, sem login).
  onEnqueueYouTubeLinks?: (text: string, playNow: boolean) => Promise<number>;
}) {
  const isPlayer = useMode() === "tocador";
  // no Tocador, Internet some; se estava nela, volta pro Operacional.
  const effMode: PanelMode = isPlayer && mode === "internet" ? "operacional" : mode;

  return (
    <aside className="rightpanel">
      <div className="rightpanel-head">
        <nav className="rightpanel-modes">
          <button
            className={"rp-mode" + (effMode === "operacional" ? " on" : "")}
            onClick={() => onMode("operacional")}
          >
            {isPlayer ? "🎧 Meu Som" : "🎛 Operacional"}
          </button>
          {!isPlayer && (
            <button
              className={"rp-mode" + (effMode === "internet" ? " on" : "")}
              onClick={() => onMode("internet")}
            >
              🌐 Internet
            </button>
          )}
        </nav>
        <button
          className="rightpanel-close"
          onClick={onClose}
          title="Fechar painel"
          aria-label="Fechar painel"
        >
          ✕
        </button>
      </div>

      <div className="rightpanel-body">
        {effMode === "operacional" ? (
          <OperacionalScreen
            onEnqueue={onEnqueue}
            onEnqueueSpotify={onEnqueueSpotify}
            onEnqueueYouTube={onEnqueueYouTube}
            onEnqueueYouTubeLinks={onEnqueueYouTubeLinks}
          />
        ) : (
          <WebPanel />
        )}
      </div>
    </aside>
  );
}
