"use client";

import { useEffect, useState } from "react";
import { useSpotify } from "@/hooks/useSpotify";
import { SpotifyResults } from "@/components/SpotifyResults";
import { SpotifySetup } from "@/components/SpotifySetup";
import { BrandIcon } from "@/components/BrandIcon";
import { SpotifyDiagnose } from "@/components/SpotifyDiagnose";
import type { CatalogItem } from "@/lib/api";

/* painel Spotify — foco no fluxo de conexão (Story 6.1–6.3). Três estados:
   (a) integração não configurada → guia de setup (SpotifySetup); (b) configurada
   mas desconectada → card hero "Conectar conta" (PKCE); (c) conectada → busca +
   import de metadados + enfileirar no playout. Faixas importadas viram Track
   SourceSpotify não-mixável (metadata only, sem PCM). O playout real da faixa
   Spotify é o deck (Web Playback SDK via useSpotifyPlayout) — sem prévia isolada
   duplicada aqui. Sem lista hardcoded — resultados vêm da Web API. */
export function SpotifyPanel({
  onEnqueue,
  onImported,
  query: queryProp,
  onQuery,
}: {
  // enfileira um resultado no playout: importa a URI e enfileira (out-of-band SDK).
  onEnqueue?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  // chamado após um import (ou enqueue, que também importa) landar no backend, pra
  // a lista de importadas recarregar — a faixa importada aparece na PRÓPRIA aba
  // Spotify (embaixo da busca), não na aba Músicas. Sem trocar de aba.
  onImported?: () => void;
  // busca CONTROLADA pelo pai (OperacionalScreen) → persiste ao trocar de aba (o
  // painel desmonta, mas a query mora no pai). Sem prop = estado local (standalone).
  query?: string;
  onQuery?: (v: string) => void;
}) {
  const {
    status,
    configured,
    loaded,
    busy,
    error,
    connect,
    disconnect,
    results,
    searching,
    imported,
    searchAll,
    importItem,
  } = useSpotify();
  // busca CONTROLADA se o pai passar query/onQuery (persiste ao trocar de aba, pois
  // a query mora no OperacionalScreen); senão estado local (standalone).
  const [queryLocal, setQueryLocal] = useState("");
  const query = queryProp ?? queryLocal;
  const setQuery = onQuery ?? setQueryLocal;

  const canSearch = status.connected && configured;
  // import de um resultado → persiste no backend, depois recarrega a biblioteca
  // (senão a faixa Spotify importada não aparece em Mídias até dar F5).
  const handleImport = async (item: CatalogItem) => {
    await importItem(item);
    onImported?.();
  };
  // menu do resultado → mesma ação do menu local; erro já toastado por quem
  // enfileira, engole aqui pra não virar unhandled rejection. Enqueue também
  // importa a faixa no backend → recarrega a biblioteca ao terminar.
  const onEnqueueItem = onEnqueue
    ? (item: CatalogItem, pos: number, playNow: boolean) =>
        void Promise.resolve(
          // playlist → nome vira origem das faixas; faixa avulsa não tem origem.
          onEnqueue(item.uri, pos, playNow, item.kind === "playlist" ? item.title : ""),
        )
          .then(() => onImported?.())
          .catch(() => {})
    : undefined;

  // busca automática ao digitar (debounce 300ms). Traz faixas E playlists de uma
  // vez — sem botão Buscar, sem escolher tipo. Query vazia limpa os resultados.
  useEffect(() => {
    if (!canSearch) return;
    const t = setTimeout(() => void searchAll(query), 300);
    return () => clearTimeout(t);
  }, [query, canSearch, searchAll]);

  // (a) integração não configurada → guia de setup
  if (loaded && !configured) return <SpotifySetup />;

  // (b) configurada mas desconectada → card hero de conexão
  if (loaded && !status.connected) {
    return (
      <div className="spot">
        <div className="spot-hero">
          <span className="spot-hero-logo">
            <BrandIcon name="spotify" size={44} />
          </span>
          <h3 className="spot-hero-title">Conectar ao Spotify</h3>
          <p className="spot-hero-sub">
            Vincule sua conta para buscar e importar faixas e playlists direto
            do catálogo do Spotify.
          </p>
          <button className="spot-hero-btn" onClick={connect} disabled={busy}>
            {busy ? "Conectando…" : "Conectar conta"}
          </button>
          {error && <div className="spot-warn spot-err">⚠️ {error}</div>}
          <p className="spot-hero-note">
            Premium só é preciso para tocar a prévia no navegador. Buscar e
            importar metadados funciona em qualquer conta.
          </p>
        </div>
      </div>
    );
  }

  // (c) conectada → busca + import + prévia
  return (
    <div className="spot">
      <div className="spot-status">
        <span className="spot-logo">
          <BrandIcon name="spotify" size={17} decorative /> Spotify
        </span>
        <span className={"spot-chip" + (status.premium ? " prem" : "")}>
          ● Conectado{status.premium ? " · Premium" : " · sem Premium"}
        </span>
        <SpotifyDiagnose />
        <button className="spot-disc" onClick={disconnect} disabled={busy}>
          Desconectar
        </button>
      </div>

      {error && <div className="spot-warn spot-err">⚠️ {error}</div>}
      {imported !== null && (
        <div className="spot-warn spot-ok">
          ✓ {imported} faixa{imported === 1 ? "" : "s"} importada
          {imported === 1 ? "" : "s"} para a biblioteca.
        </div>
      )}

      <div className="spot-search">
        <span className="spot-search-ico">{searching ? "⏳" : "🔍"}</span>
        <input
          className="spot-input"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Digite para buscar faixas e playlists no Spotify…"
          autoFocus
        />
        {query && (
          <button
            className="spot-clear"
            onClick={() => setQuery("")}
            title="Limpar"
          >
            ✕
          </button>
        )}
      </div>

      {!status.premium && (
        <div className="spot-note">
          ▶ Tocar a faixa no deck (Web Playback SDK) requer <b>Spotify Premium</b>.
          Sem Premium você ainda busca e importa metadados.
        </div>
      )}

      {results.length > 0 && (
        <SpotifyResults
          results={results}
          busy={busy}
          onImport={handleImport}
          onEnqueue={onEnqueueItem}
        />
      )}

      {!searching && results.length === 0 && query !== "" && (
        <div className="spot-empty">Nenhum resultado.</div>
      )}

      <div className="spot-note">
        🎧 Importação traz só <b>metadados</b> (título/artista/álbum). O Spotify
        não expõe o áudio (DRM) — BPM/refrão/waveform ficam pros arquivos locais.
      </div>
    </div>
  );
}
