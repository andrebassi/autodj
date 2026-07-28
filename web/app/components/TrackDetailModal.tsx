"use client";

import type { OpMedia } from "@/lib/operational";
import { TrackCover } from "@/components/TrackCover";
import { SourceBadge } from "@/components/SourceBadge";

/* Popup de detalhes da faixa: capa ampliada + tudo que sabemos dela (plataforma,
   duração, BPM, tom, mixável, análise, marcadores, tocadas, deep link Spotify).
   Abre ao clicar na capa na lista de Mídias. Puramente informativo — não toca em
   fila/broadcast. Campos ausentes são omitidos (não polui com "—" em tudo). */

/* spotify:track:ID → https://open.spotify.com/track/ID (abre no app/web). */
function spotifyWebURL(uri?: string): string | null {
  if (!uri) return null;
  const m = uri.match(/^spotify:track:([A-Za-z0-9]+)$/);
  return m ? "https://open.spotify.com/track/" + m[1] : null;
}

function fmtDur(ms?: number): string | null {
  if (!ms || ms <= 0) return null;
  const s = Math.round(ms / 1000);
  const mm = Math.floor(s / 60);
  const ss = s % 60;
  return `${mm}:${String(ss).padStart(2, "0")}`;
}

/* rótulos pt-BR das categorias conhecidas (fallback = a própria chave). */
const CAT_LABEL: Record<string, string> = {
  music: "Música",
  musicas: "Música",
  commercial: "Comercial",
  comerciais: "Comercial",
  jingle: "Vinheta",
  vinhetas: "Vinheta",
  spotify: "Spotify",
};

export function TrackDetailModal({
  media,
  onClose,
}: {
  media: OpMedia;
  onClose: () => void;
}) {
  const m = media;
  const spotUrl = spotifyWebURL(m.spotifyUri);
  // Plataforma: selo elegante (logo+wordmark) p/ Spotify/YouTube; local mantém
  // a descrição técnica (PCM) — mais honesto que só "Local" no modal de detalhes.
  const platform: React.ReactNode =
    m.source === "spotify" || m.source === "youtube" ? (
      <SourceBadge source={m.source} />
    ) : (
      "Arquivo local (PCM)"
    );

  // linhas (rótulo, valor) — só entram as que têm valor.
  const rows: [string, React.ReactNode][] = [];
  const push = (label: string, val: React.ReactNode) => {
    if (val !== null && val !== undefined && val !== "") rows.push([label, val]);
  };

  push("Plataforma", platform);
  push("Intérprete", m.artist && m.artist !== "—" ? m.artist : null);
  push("Álbum", m.album || null);
  push("Categoria", m.category ? CAT_LABEL[m.category] ?? m.category : null);
  push("Duração", fmtDur(m.durationMs) ?? (m.dur && m.dur !== "00:00" ? m.dur : null));
  push("BPM", m.bpm && m.bpm > 0 ? Math.round(m.bpm) : null);
  push("Tom (Camelot)", m.musicalKey || null);
  // "Mixável" saiu da tela na 0.2.14 (pedido do usuário): a mixabilidade é
  // consequência da plataforma (Spotify/YouTube nunca; local sempre) e repeti-la
  // em toda linha/detalhe era ruído. A regra segue valendo no domínio.
  push(
    "Análise",
    m.source === "spotify"
      ? "N/A (sem PCM)"
      : m.analyzed === undefined
        ? null
        : m.analyzed
          ? "Concluída"
          : "Pendente",
  );
  push("Marcadores", m.cues && m.cues.length > 0 ? String(m.cues.length) : null);
  push("Tocadas", m.playCount && m.playCount > 0 ? String(m.playCount) : null);
  push("ID", m.id || null);

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div
        className="mix-modal td-modal"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="mix-titlebar">Detalhes da faixa</header>
        <div className="td-body">
          <div className="td-cover">
            <TrackCover trackId={m.id} src={m.artworkUrl} size="deck" alt={m.title} />
          </div>
          <div className="td-head">
            <div className="td-title" title={m.title}>
              {m.title}
            </div>
            <div className="td-artist">{m.artist}</div>
            <span
              className={
                "td-badge" + (m.source === "spotify" ? " td-badge-spot" : "")
              }
            >
              {platform}
            </span>
          </div>
        </div>

        <table className="td-tbl">
          <tbody>
            {rows.map(([label, val]) => (
              <tr key={label}>
                <td className="td-k">{label}</td>
                <td className="td-v">{val}</td>
              </tr>
            ))}
          </tbody>
        </table>

        <footer className="mix-footer">
          {spotUrl && (
            <a
              className="mix-save td-open"
              href={spotUrl}
              target="_blank"
              rel="noopener noreferrer"
            >
              Abrir no Spotify
            </a>
          )}
          <button className="mix-close" onClick={onClose}>
            Fechar
          </button>
        </footer>
      </div>
    </div>
  );
}
