"use client";

import { useEffect, useState, type CSSProperties } from "react";
import { useYouTube } from "@/hooks/useYouTube";
import { YouTubeResults } from "@/components/YouTubeResults";
import { YouTubeSetup } from "@/components/YouTubeSetup";
import { YouTubeOriginWarning } from "@/components/YouTubeOriginWarning";
import { YouTubeLinksImport } from "@/components/YouTubeLinksImport";
import { YouTubeBackendSwitch } from "@/components/YouTubeBackendSwitch";
import { BrandIcon } from "@/components/BrandIcon";
import type { YouTubeBackend, YouTubeItem } from "@/lib/api";

type Sub = "buscar" | "colar";

/* painel YouTube — duas sub-abas, e a INTEGRAÇÃO NÃO É OBRIGATÓRIA (0.2.17):

   - "Colar links" (sem login): textarea grande p/ colar endereços separados por ";"
     → importa direto pra fila/deck. Resolver ids custa 1 unidade de cota por lote.
   - "Buscar": DOIS motores, chaveáveis no `YouTubeBackendSwitch` — "Sem login"
     (InnerTube, 0 unidade de cota, nenhuma conta) e "Minha conta" (Data API,
     `search.list` ~100 unidades das ~10k/dia, exige OAuth). O login do Google segue
     sendo o ESCUDO DE COTA do caminho pago: prod é público e multi-tenant por
     fingerprint; sem gate ali, um visitante queimaria a cota do dia de todo mundo em
     ~100 teclas. Nada foi removido — a Data API continua disponível a um clique.

   Antes o painel inteiro ficava atrás do card "Conectar ao YouTube" e não dava pra
   fazer NADA sem OAuth. Agora o hero de conexão vive só dentro da sub-aba Buscar (e
   só quando o motor escolhido é o da Data API), e desconectado o painel abre já em
   "Colar links".

   Faixas importadas viram Track SourceYouTube não-mixável (metadata only, sem PCM);
   o playout real é o deck (IFrame Player API via useYouTubePlayout). */
export function YouTubePanel({
  onEnqueue,
  onEnqueueLinks,
  onImported,
  query: queryProp,
  onQuery,
}: {
  // enfileira um resultado no playout: importa a URI e enfileira (out-of-band iframe).
  onEnqueue?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  // importa o texto colado E enfileira as faixas; resolve com quantas entraram. Sem
  // essa prop (painel standalone), cai no importLinks do hook — que só importa pra
  // biblioteca, sem enfileirar.
  onEnqueueLinks?: (text: string, playNow: boolean) => Promise<number>;
  // chamado após um import (ou enqueue, que também importa) landar no backend, pra
  // a lista de importadas recarregar — a faixa importada aparece na PRÓPRIA aba
  // YouTube (embaixo da busca), não na aba Músicas. Sem trocar de aba.
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
    importLinks,
  } = useYouTube();
  // busca CONTROLADA se o pai passar query/onQuery (persiste ao trocar de aba, pois
  // a query mora no OperacionalScreen); senão estado local (standalone).
  const [queryLocal, setQueryLocal] = useState("");
  const query = queryProp ?? queryLocal;
  const setQuery = onQuery ?? setQueryLocal;

  // sub-aba: null = ninguém escolheu ainda → abre no que funciona AGORA. Desde que
  // o motor grátis existe, "Buscar" funciona deslogado, então ele é o default: abrir
  // em "Colar links" escondia o chaveador atrás de dois cliques não-óbvios (o painel
  // já abre no Spotify) e dava a impressão de que a busca sem login não tinha subido.
  // Só cai em "Colar links" quando buscar é impossível — servidor sem o motor grátis
  // e sem conta ligada —, porque colar link só precisa da API key. Escolha manual trava.
  const [subPick, setSubPick] = useState<Sub | null>(null);
  const sub: Sub = subPick ?? (status.freeSearch || status.connected ? "buscar" : "colar");

  // motor da busca. Default "free" (InnerTube: sem conta, sem cota). `searchBackend`
  // é o EFETIVO: servidor sem o backend grátis ligado só tem a Data API, então a
  // escolha do operador é ignorada e cai em "api" (o switcher nem aparece).
  const [backend, setBackend] = useState<YouTubeBackend>("free");
  // Sem OAuth no servidor o motor "Minha conta" é impossível (connect → 503), então
  // some o chaveador e a busca fica só no grátis — oferecer um caminho que termina em
  // erro é pior que não oferecer. Com OAuth, os dois motores como sempre.
  const canChooseBackend = !!status.freeSearch && !!status.oauth;
  const searchBackend: YouTubeBackend = !status.freeSearch
    ? "api"
    : canChooseBackend
      ? backend
      : "free";

  // conta só é pré-requisito no caminho da Data API — o grátis busca deslogado.
  const canSearch =
    configured &&
    (searchBackend === "free" ? !!status.freeSearch : status.connected);
  // import de um resultado → persiste no backend, depois recarrega a biblioteca
  // (senão a faixa YouTube importada não aparece em Mídias até dar F5).
  const handleImport = async (item: YouTubeItem) => {
    await importItem(item);
    onImported?.();
  };
  // menu do resultado → mesma ação do menu local; erro já toastado por quem
  // enfileira, engole aqui pra não virar unhandled rejection. Enqueue também
  // importa a faixa no backend → recarrega a biblioteca ao terminar.
  const onEnqueueItem = onEnqueue
    ? (item: YouTubeItem, pos: number, playNow: boolean) =>
        void Promise.resolve(
          // playlist → nome vira origem das faixas; vídeo avulso não tem origem.
          onEnqueue(item.uri, pos, playNow, item.kind === "playlist" ? item.title : ""),
        )
          .then(() => onImported?.())
          .catch(() => {})
    : undefined;

  // paste → enfileira (tela operacional) ou só importa (standalone). Nos dois casos
  // recarrega a biblioteca ao terminar.
  const handleLinks = async (text: string, playNow: boolean): Promise<number> => {
    const n = onEnqueueLinks
      ? await onEnqueueLinks(text, playNow)
      : await importLinks(text);
    if (n > 0) onImported?.();
    return n;
  };

  // busca automática ao digitar (debounce 300ms). Traz vídeos E playlists de uma
  // vez — sem botão Buscar, sem escolher tipo. Query vazia limpa os resultados.
  // trocar de motor com a busca preenchida refaz a consulta (searchBackend nas deps).
  useEffect(() => {
    if (!canSearch || sub !== "buscar") return;
    const t = setTimeout(() => void searchAll(query, searchBackend), 300);
    return () => clearTimeout(t);
  }, [query, canSearch, sub, searchAll, searchBackend]);

  // integração não configurada (backend sem API key nem client OAuth) → guia de setup
  if (loaded && !configured) return <YouTubeSetup />;

  const glide: CSSProperties = {
    width: "calc((100% - 8px) / 2)",
    transform: `translateX(${sub === "buscar" ? 0 : 100}%)`,
  };

  return (
    <div className="spot">
      <YouTubeOriginWarning />

      <div className="spot-status">
        <span className="spot-logo">
          <BrandIcon name="youtube" size={17} decorative /> YouTube
        </span>
        {status.connected ? (
          <>
            <span className="spot-chip">● Conectado</span>
            <button className="spot-disc" onClick={disconnect} disabled={busy}>
              Desconectar
            </button>
          </>
        ) : (
          <span className="spot-chip spot-chip-off">○ Sem conta (opcional)</span>
        )}
      </div>

      <div className="opsub2 ptabs-nav" role="tablist" aria-label="Buscar ou colar links">
        <span className="opsub2-glide" style={glide} aria-hidden />
        <button
          type="button"
          role="tab"
          aria-selected={sub === "buscar"}
          className={"opsub2-tab" + (sub === "buscar" ? " on" : "")}
          onClick={() => setSubPick("buscar")}
        >
          <span className="opsub2-ico">🔍</span>
          <span className="opsub2-lbl">Buscar</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={sub === "colar"}
          className={"opsub2-tab" + (sub === "colar" ? " on" : "")}
          onClick={() => setSubPick("colar")}
        >
          <span className="opsub2-ico">🔗</span>
          <span className="opsub2-lbl">Colar links</span>
        </button>
      </div>

      {error && <div className="spot-warn spot-err">⚠️ {error}</div>}

      {sub === "colar" ? (
        <YouTubeLinksImport onImport={handleLinks} />
      ) : (
        <>
          {canChooseBackend && (
            <YouTubeBackendSwitch
              value={backend}
              onChange={setBackend}
              connected={status.connected}
            />
          )}

          {searchBackend === "api" && !status.connected ? (
            // só o motor da Data API exige conta: escudo de cota do search.list
            <div className="spot-hero">
              <span className="spot-hero-logo">
                <BrandIcon name="youtube" size={44} />
              </span>
              <h3 className="spot-hero-title">Conectar ao YouTube</h3>
              <p className="spot-hero-sub">
                A busca <b>pela sua conta</b> usa a API oficial do Google — o login é
                o que segura a cota diária (~100 buscas). Para buscar sem conta,
                escolha <b>⚡ Sem login</b> acima; para adicionar vídeos por URL, use
                a aba <b>Colar links</b>.
              </p>
              <button className="spot-hero-btn" onClick={connect} disabled={busy}>
                {busy ? "Conectando…" : "Conectar com Google"}
              </button>
              <p className="spot-hero-note">
                O player toca no navegador via YouTube (grátis, sem Premium). Buscar e
                importar metadados funciona em qualquer conta Google.
              </p>
            </div>
          ) : (
            <>
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
                  placeholder="Digite para buscar vídeos e playlists no YouTube…"
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

              {results.length > 0 && (
                <YouTubeResults
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
                🎧 Importação traz só <b>metadados</b> (título/canal/thumbnail). O
                YouTube não expõe o áudio (embed/DRM) — BPM/refrão/waveform ficam pros
                arquivos locais. A faixa toca no deck via IFrame Player API (60×60), no
                mesmo tamanho da capa.
                {searchBackend === "free" && (
                  <>
                    {" "}
                    Na busca <b>⚡ sem login</b> os resultados vêm direto do YouTube; se
                    um vídeo não permitir incorporação, ele é descartado na importação.
                  </>
                )}
              </div>
            </>
          )}
        </>
      )}
    </div>
  );
}
