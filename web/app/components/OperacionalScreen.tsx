"use client";

import { useEffect, useState } from "react";
import { type OpCategoryKey } from "@/lib/operational";
import { useMode } from "@/lib/mode";
import { useOperational } from "@/hooks/useOperational";
import { useLibrary } from "@/hooks/useLibrary";
import { AboutAutoDJ } from "@/components/AboutAutoDJ";
import { OpCategoryBar } from "@/components/OpCategoryBar";
import { OpSearch } from "@/components/OpSearch";
import { FolderBrowser } from "@/components/FolderBrowser";
import { useFolders } from "@/hooks/useFolders";
import { useInstrumental } from "@/hooks/useInstrumental";
import { useServerLinked } from "@/hooks/useServerLinked";
import { OpMediaList } from "@/components/OpMediaList";
import { useStemWork } from "@/hooks/useStemWork";
import { MarkersEditor } from "@/components/MarkersEditor";
import { Cartwall } from "@/components/Cartwall";
import { SpotifyPanel } from "@/components/SpotifyPanel";
import { YouTubePanel } from "@/components/YouTubePanel";
import { AIDJPanel } from "@/components/AIDJPanel";
import { MixConfigModal } from "@/components/MixConfigModal";
import { ClocksModal } from "@/components/ClocksModal";
import { GradeModal } from "@/components/GradeModal";
import { EventsModal } from "@/components/EventsModal";
import { RDSModal } from "@/components/RDSModal";
import { LineInPanel } from "@/components/LineInPanel";
import { PlayLogPanel } from "@/components/PlayLogPanel";
import { ECADReportPanel } from "@/components/ECADReportPanel";
import { RequestsPanel } from "@/components/RequestsPanel";
import { PublicSitePanel } from "@/components/PublicSitePanel";
import { SocialPanel } from "@/components/SocialPanel";
import { StaffSchedulePanel } from "@/components/StaffSchedulePanel";
import { BroadcastProofPanel } from "@/components/BroadcastProofPanel";
import { UploadModal } from "@/components/UploadModal";
import { TrackFormModal } from "@/components/TrackFormModal";
import { setTrackFolders } from "@/lib/api";
import { confirmDialog } from "@/lib/confirm";
import { toast } from "@/lib/toast";

/* categorias que sobrevivem no modo Tocador (tocador puro de música). As demais
   (vinhetas/trilhas/extras/comerciais/programetes/outros/oferecimentos/info
   cantores/aniversários) são funções de estação — somem no Tocador. */
const PLAYER_CATS = new Set<OpCategoryKey>([
  "sobre",
  "instantaneas",
  "musicas",
  "spotify",
  "youtube",
]);

/* Abas que NÃO são categoria de mídia (não têm busca, pasta nem lista de faixas).
   Existia como uma corrente de `cat !== "x" && cat !== "y"` repetida em três
   lugares; com a aba de apresentação seriam quatro comparações em cada uma, e é
   exatamente o tipo de lista que alguém esquece de atualizar num dos pontos. */
const NON_MEDIA_CATS = new Set<OpCategoryKey>([
  "sobre",
  "instantaneas",
  "spotify",
  "youtube",
]);

/* tela Operacional — categorias + biblioteca + editor + Spotify.
   onEnqueue liga a lista de mídias à fila real do Player (menu + drag). */
export function OperacionalScreen({
  onEnqueue,
  onEnqueueSpotify,
  onEnqueueYouTube,
  onEnqueueYouTubeLinks,
}: {
  onEnqueue?: (trackId: string, pos: number, playNow: boolean) => void;
  onEnqueueSpotify?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  onEnqueueYouTube?: (uri: string, pos: number, playNow: boolean, origin?: string) => void;
  // enfileira um LOTE de links colados (sub-aba "Colar links" do YouTube, sem login).
  onEnqueueYouTubeLinks?: (text: string, playNow: boolean) => Promise<number>;
}) {
  // modo Tocador esconde as funções de rádio e as categorias de estação, deixando
  // só o essencial de tocador de música.
  const mode = useMode();
  const isPlayer = mode === "tocador";
  /* ABRE NA APRESENTAÇÃO (pedido do operador): a primeira coisa ao entrar é o que
     este app é, não o card de login de um serviço externo.
     Ela NÃO tem chip na barra (pedido do operador): é tela de entrada, não uma
     aba. Some no primeiro clique em qualquer categoria e não volta — por isso
     `active={cat}` simplesmente não casa com chip nenhum enquanto ela está no ar,
     e a barra fica sem nada aceso, que é o desejado. */
  const [cat, setCat] = useState<OpCategoryKey>("sobre");
  // Pasta selecionada ("" = todas). Mora aqui, e não dentro do navegador de pastas,
  // pra sobreviver a re-render/troca de categoria: o operador filtra por "rock", vai
  // no Spotify e volta esperando o filtro ainda de pé.
  const [folder, setFolder] = useState("");
  const fold = useFolders();
  // separação de vocal por IA: pedido + acompanhamento (pré-render, leva minutos)
  const stem = useInstrumental();
  /* O que está sendo separado AGORA — uma consulta para a lista inteira, e o poll
     só vive enquanto há trabalho (ver o hook). É o que desenha a barra na linha. */
  const stemWork = useStemWork();
  // enlace com o servidor: o que é 100% online (Spotify, YouTube, envio) fica
  // apagado enquanto ele não responde, e reacende sozinho quando volta.
  const linked = useServerLinked();
  const [sel, setSel] = useState(0);
  const [mixOpen, setMixOpen] = useState(false);
  const [clocksOpen, setClocksOpen] = useState(false);
  const [gradeOpen, setGradeOpen] = useState(false);
  const [eventsOpen, setEventsOpen] = useState(false);
  const [rdsOpen, setRdsOpen] = useState(false);
  const [lineInOpen, setLineInOpen] = useState(false);
  const [playLogOpen, setPlayLogOpen] = useState(false);
  const [ecadOpen, setEcadOpen] = useState(false);
  const [proofOpen, setProofOpen] = useState(false);
  const [requestsOpen, setRequestsOpen] = useState(false);
  const [publicSiteOpen, setPublicSiteOpen] = useState(false);
  const [socialOpen, setSocialOpen] = useState(false);
  const [staffOpen, setStaffOpen] = useState(false);
  const [aidjOpen, setAidjOpen] = useState(false);
  const [markersFor, setMarkersFor] = useState<number | null>(null); // modal oculto por padrão
  const [q, setQ] = useState("");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [editFor, setEditFor] = useState<number | null>(null);
  const { op } = useOperational();
  const { tracks, loaded, categories, upload, update, remove, reload } = useLibrary();
  const hasLocal = tracks.some((t) => t.source !== "spotify" && t.source !== "youtube");

  // no Tocador, filtra os chips de categoria de estação; se a categoria atual sumir,
  // volta pra "musicas" pra não ficar preso numa aba invisível.
  const visibleCats = isPlayer
    ? op.categories.filter((c) => PLAYER_CATS.has(c.key as OpCategoryKey))
    : op.categories;
  useEffect(() => {
    if (isPlayer && !PLAYER_CATS.has(cat)) setCat("musicas");
  }, [isPlayer, cat]);

  // aba Músicas só existe se houver faixa LOCAL. Sem nenhuma, o chip fica
  // desabilitado (pedido do usuário) e, se estiver ativo numa categoria de mídia
  // vazia, cai pra aba Spotify — a que tem conteúdo importável de cara.
  const mediaCat = !NON_MEDIA_CATS.has(cat);
  useEffect(() => {
    // ⚠️ NUNCA cair pro Spotify SEM SERVIDOR (0.2.27): offline ele é justamente o
    // único que não funciona, e a queda também zera `hasLocal` no primeiro instante
    // — o app abria na aba mais inútil possível, com a biblioteca local intacta ao
    // lado.
    if (linked && loaded && !hasLocal && mediaCat) setCat("spotify");
  }, [linked, loaded, hasLocal, mediaCat]);

  /* Sem servidor, Spotify/YouTube/Adicionar só podem falhar: busca, import e
     upload são todos API. Apagados enquanto não há enlace e reacesos sozinhos
     quando volta — mais honesto que um clique que termina em erro. A aba Músicas
     fica viva porque lê do espelho local e toca do cache. */
  useEffect(() => {
    if (!linked && (cat === "spotify" || cat === "youtube")) setCat("musicas");
  }, [linked, cat]);

  // Retorno do OAuth do Spotify/YouTube: o callback redireciona pra /?op=spotify ou
  // /?op=youtube — abre direto a categoria correspondente (a tela onde a conta acabou
  // de conectar) e limpa o query param pra não reabrir em navegações futuras. Só no boot.
  useEffect(() => {
    const sp = new URLSearchParams(window.location.search);
    const target = sp.get("op");
    if (target === "spotify" || target === "youtube") {
      setCat(target);
      sp.delete("op");
      const qs = sp.toString();
      window.history.replaceState(null, "", window.location.pathname + (qs ? "?" + qs : ""));
    }
  }, []);

  // busca acento-insensível por intérprete/título
  const norm = (s: string) => s.normalize("NFD").replace(/\p{Diacritic}/gu, "").toLowerCase();
  const nq = norm(q.trim());
  const match = (t: (typeof tracks)[number]) => norm(`${t.artist} ${t.title}`).includes(nq);
  // ABAS SEPARADAS por fonte: Músicas = só locais; Spotify = só do Spotify;
  // YouTube = só do YouTube. Nunca juntar — a faixa importada aparece na PRÓPRIA aba
  // da fonte, embaixo da busca (não some pra Músicas). A busca `q` só vale na aba
  // Músicas.
  const localAll = tracks.filter((t) => t.source !== "spotify" && t.source !== "youtube");
  const spotAll = tracks.filter((t) => t.source === "spotify");
  const ytAll = tracks.filter((t) => t.source === "youtube");
  // Dentro de uma PASTA o universo é a biblioteca INTEIRA, não só as locais: a pasta
  // "Spotify" mostra as do Spotify e a "YouTube" as do YouTube — foi o pedido. Em
  // "Todas" (sem pasta escolhida) o recorte por fonte continua valendo, senão a aba
  // Músicas passaria a listar faixa de streaming.
  const inFolder = (t: (typeof tracks)[number]) =>
    !folder || (t.folders ?? []).some((f) => f.toLowerCase() === folder.toLowerCase());
  const universe = folder ? tracks : localAll;
  const localMedia = (nq ? universe.filter(match) : universe).filter(inFolder);
  // lista que os handlers indexados (sel/markers/edit/delete) enxergam = a da aba ativa
  const media = cat === "spotify" ? spotAll : cat === "youtube" ? ytAll : localMedia;
  const mkMedia = markersFor != null ? media[markersFor] : null;
  const editMedia = editFor != null ? media[editFor] : null;

  const handleEnqueue = async (trackId: string, pos: number, playNow: boolean) => {
    const t = media.find((m) => m.id === trackId);
    // Spotify entra na fila como cidadão de 1ª classe: toca out-of-band pelo
    // SDK do browser (hard_cut). Só o bus PCM/mix local (Liquidsoap) o recusa.
    try {
      await onEnqueue?.(trackId, pos, playNow);
      if (t) toast.success(playNow ? `Tocando "${t.title}"` : `"${t.title}" na fila`);
    } catch {
      /* wrapper já toastou o erro */
    }
  };

  // pós-import do Spotify (ou enqueue, que também importa): só recarrega a
  // biblioteca. A faixa importada aparece na PRÓPRIA aba Spotify (lista embaixo da
  // busca) — sem trocar de aba, sem misturar com as locais da aba Músicas.
  const handleImported = () => void reload();

  const askDelete = async (i: number) => {
    const m = media[i];
    if (!m?.id) return;
    const ok = await confirmDialog({
      title: `Apagar "${m.title}"?`,
      body: "Ação irreversível.",
      okText: "Apagar",
      danger: true,
    });
    if (!ok) return;
    await remove(m.id);
    toast.success(`"${m.title}" apagada.`);
  };

  return (
    <div className="opscreen">
      <div className="opscreen-head">
        <button
          className="opscreen-add"
          onClick={() => setUploadOpen(true)}
          disabled={!linked}
          title={linked ? undefined : "Sem conexão com o servidor — envio indisponível"}
        >
          ＋ Adicionar
        </button>
        {/* funções de estação — só no modo Rádio (Mixagem/Tocadas inclusas) */}
        {!isPlayer && (
          <>
            <button className="opscreen-mix" onClick={() => setMixOpen(true)}>
              🎚 Mixagem
            </button>
            <button className="opscreen-mix" onClick={() => setClocksOpen(true)}>
              🕐 Relógios
            </button>
            <button className="opscreen-mix" onClick={() => setGradeOpen(true)}>
              🗓 Grade
            </button>
            <button className="opscreen-mix" onClick={() => setEventsOpen(true)}>
              📅 Eventos
            </button>
            <button className="opscreen-mix" onClick={() => setRdsOpen(true)}>
              📡 RDS
            </button>
            <button className="opscreen-mix" onClick={() => setLineInOpen(true)}>
              🎙 Line-in
            </button>
          </>
        )}
        {!isPlayer && (
          <>
            <button className="opscreen-mix" onClick={() => setPlayLogOpen(true)}>
              📊 Tocadas
            </button>
            <button className="opscreen-mix" onClick={() => setEcadOpen(true)}>
              🎼 ECAD
            </button>
            <button className="opscreen-mix" onClick={() => setProofOpen(true)}>
              📻 Comprovante
            </button>
            <button className="opscreen-mix" onClick={() => setRequestsOpen(true)}>
              📨 Pedidos
            </button>
            <button className="opscreen-mix" onClick={() => setPublicSiteOpen(true)}>
              🌐 Site público
            </button>
            <button className="opscreen-mix" onClick={() => setSocialOpen(true)}>
              📣 Redes sociais
            </button>
            <button className="opscreen-mix" onClick={() => setStaffOpen(true)}>
              🗂 Escala
            </button>
          </>
        )}
        {/* Locutor com IA — desabilitado por ora (pedido do usuário). Fica VISÍVEL
            de propósito (o usuário quer ver a função existindo), só não clica. */}
        <button
          className="opscreen-mix"
          onClick={() => setAidjOpen(true)}
          disabled
          title="Locutor com IA — em breve"
        >
          🎙 Locutor com IA
        </button>
      </div>

      <OpCategoryBar
        categories={visibleCats}
        active={cat}
        onPick={setCat}
        // "instantaneas" (cartwall) fica SEMPRE visível e SEMPRE desligada por ora
        // (pedido do usuário) — igual ao botão Locutor com IA. "musicas" só liga
        // quando existe faixa local.
        // Sem servidor, Spotify e YouTube entram na lista: são 100% online (busca,
        // import e playout passam pela API ou pelo provedor), então ficam apagados
        // e voltam sozinhos quando o enlace retorna.
        disabledKeys={
          new Set<OpCategoryKey>([
            "instantaneas",
            ...(hasLocal ? [] : (["musicas"] as OpCategoryKey[])),
            ...(linked ? [] : (["spotify", "youtube"] as OpCategoryKey[])),
          ])
        }
        disabledTitles={{
          instantaneas: "Instantâneas — em breve",
          musicas: "Adicione uma música local para habilitar",
          spotify: "Sem conexão com o servidor",
          youtube: "Sem conexão com o servidor",
        }}
      />

      {/* ---- SOBRE: apresentação do app (aba de entrada) ---- */}
      {cat === "sobre" && (
        <div className="panel-slot" data-panel="sobre">
          <AboutAutoDJ
            hasLocal={hasLocal}
            onStart={() => setCat("musicas")}
            onAdd={() => setUploadOpen(true)}
          />
        </div>
      )}

      {/* ---- INSTANTÂNEAS: cartwall ---- */}
      {cat === "instantaneas" && (
        <div className="panel-slot" data-panel="cartwall">
          <Cartwall />
        </div>
      )}

      {/* ---- SPOTIFY: conexão + busca/import. A lista das faixas já importadas
           fica oculta por ora (pedido do usuário). Import segue gravando no
           banco; só não renderiza a lista aqui. ---- */}
      {cat === "spotify" && (
        <div className="panel-slot" data-panel="spotify">
          <SpotifyPanel
            onEnqueue={onEnqueueSpotify}
            onImported={handleImported}
            query={q}
            onQuery={setQ}
          />
        </div>
      )}

      {/* ---- YOUTUBE: conexão (OAuth Google) + busca/import. Espelho do Spotify.
           Lista das importadas oculta por ora; import grava no banco. Playout roda
           no deck via IFrame Player API (mesmo slot 60×60). ---- */}
      {cat === "youtube" && (
        <div className="panel-slot" data-panel="youtube">
          <YouTubePanel
            onEnqueue={onEnqueueYouTube}
            onEnqueueLinks={onEnqueueYouTubeLinks}
            onImported={handleImported}
            query={q}
            onQuery={setQ}
          />
        </div>
      )}

      {/* ---- MÚSICAS (e demais categorias de mídia): biblioteca SÓ LOCAL ---- */}
      {mediaCat && (
        <>
          <OpSearch value={q} onChange={setQ} count={localMedia.length} total={localAll.length} />

          {/* Músicas abre DIRETO nas pastas, em "Todas" (0.2.26). As sub-abas
              Favoritas/Últimas Inseridas/Manuais foram removidas — nunca saíram do
              papel e roubavam o clique do que o operador realmente usa: a pasta. */}
          <FolderBrowser
            folders={fold.folders}
            selected={folder}
            onPick={setFolder}
            onCreate={fold.create}
            onDelete={async (n) => {
              await fold.remove(n);
              if (folder === n) setFolder(""); // não ficar filtrando por pasta morta
              reload();
            }}
          />

          <div className="panel-slot" data-panel="biblioteca">
            <OpMediaList
              working={stemWork.work}
              items={localMedia}
              selected={sel}
              onSelect={setSel}
              onEnqueue={handleEnqueue}
              onOpenMarkers={setMarkersFor}
              /* Separação por IA. `available === false` = ferramenta não
                 instalada nesta máquina (a rota respondeu 404) → o item some do
                 menu em vez de virar um clique que só pode falhar. */
              onInstrumental={
                stem.available === false
                  ? undefined
                  : (i) => {
                      void stem.prepare(
                        localMedia[i]?.id ?? "",
                        localMedia[i]?.title,
                      );
                      // pergunta JÁ: sem isto a barra só apareceria no próximo
                      // ciclo do poll, e o operador ficaria sem retorno no clique.
                      stemWork.refresh();
                    }
              }
              /* O estado vem de DUAS fontes: o que foi pedido nesta sessão (o
                 hook) e o que já estava pronto no catálogo (a faixa). Sem a
                 segunda, um instrumental preparado ontem apareceria como
                 "nunca pedido" depois de recarregar a página. */
              instrumentalOf={(m) =>
                stem.status[m.id ?? ""] ?? (m.instrumentalReady ? "ready" : "idle")
              }
              onEdit={setEditFor}
              onDelete={askDelete}
            />
          </div>
        </>
      )}

      {mkMedia && (
        <MarkersEditor
          palette={op.markers}
          media={mkMedia}
          onClose={() => setMarkersFor(null)}
          onSaved={reload}
        />
      )}

      {mixOpen && <MixConfigModal mix={op.mix} onClose={() => setMixOpen(false)} />}

      {clocksOpen && (
        <ClocksModal categories={categories} onClose={() => setClocksOpen(false)} />
      )}

      {gradeOpen && <GradeModal onClose={() => setGradeOpen(false)} />}

      {eventsOpen && (
        <EventsModal categories={categories} onClose={() => setEventsOpen(false)} />
      )}

      {rdsOpen && <RDSModal onClose={() => setRdsOpen(false)} />}
      {lineInOpen && <LineInPanel onClose={() => setLineInOpen(false)} />}
      {playLogOpen && <PlayLogPanel onClose={() => setPlayLogOpen(false)} />}
      {ecadOpen && <ECADReportPanel onClose={() => setEcadOpen(false)} />}
      {proofOpen && <BroadcastProofPanel onClose={() => setProofOpen(false)} />}
      {requestsOpen && <RequestsPanel onClose={() => setRequestsOpen(false)} />}
      {publicSiteOpen && <PublicSitePanel onClose={() => setPublicSiteOpen(false)} />}
      {socialOpen && <SocialPanel onClose={() => setSocialOpen(false)} />}
      {staffOpen && <StaffSchedulePanel onClose={() => setStaffOpen(false)} />}
      {aidjOpen && <AIDJPanel onClose={() => setAidjOpen(false)} />}

      {uploadOpen && (
        <UploadModal
          categories={categories}
          folders={fold.folders}
          onCreateFolder={fold.create}
          onDone={() => {
            reload(); // faixas novas na lista
            fold.reload(); // e a contagem das pastas
          }}
          onClose={() => setUploadOpen(false)}
        />
      )}

      {editMedia && (
        <TrackFormModal
          media={editMedia}
          categories={categories}
          folders={fold.folders}
          onCreateFolder={fold.create}
          onSave={update}
          onSaveFolders={async (id, fs) => {
            await setTrackFolders(id, fs);
            reload(); // a lista carrega `folders` da faixa; sem isso o filtro fica velho
            fold.reload(); // e as contagens das pastas também
          }}
          onClose={() => setEditFor(null)}
        />
      )}
    </div>
  );
}
