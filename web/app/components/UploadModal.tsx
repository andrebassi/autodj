"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { uploadTrackProgress, type CategoryDTO, type FolderDTO } from "@/lib/api";
import { toast } from "@/lib/toast";

/* Adicionar mídia — envia UM OU VÁRIOS arquivos locais (viram Tracks PCM).
   Só cria fonte local: o invariante dois-motores (spotify ⇒ !mixable) segue
   intacto porque upload nunca é Spotify.

   Por que virou lote: o acervo de rádio chega em pasta, não em arquivo. Enviar 40
   MP3s um a um — escolher, preencher, confirmar, reabrir — é o tipo de trabalho que
   faz o operador desistir e largar a biblioteca pela metade.

   Decisões que moldam a tela:
   - **Metadados por LOTE, não por arquivo.** Categoria (e opcionalmente intérprete/
     álbum) valem pra todos; o título sai do nome do arquivo. Formulário por arquivo
     em 40 itens seria pior que o problema original. Ajustes finos ficam pra edição
     depois, que já existe.
   - **Uma falha não derruba o lote.** Cada arquivo tem seu estado; quem falhar fica
     listado com o motivo e um botão de tentar de novo só dele.
   - **Concorrência 2.** Serial demora à toa; tudo de uma vez satura o upstream de
     casa e faz todas as barras rastejarem juntas.
   - **Progresso REAL** (não spinner), via XHR — `fetch` não expõe progresso de
     upload, e é o que distingue "enviando" de "travou".

   Desenho do progresso (v0.2.26 — o traço de 2px no rodapé de cada linha caiu):
   com 80+ arquivos, uma barrinha por linha vira uma cerca de tracinhos verdes e a
   tela fica suja. Agora o progresso mora em DOIS lugares proporcionais à distância
   do olho: um **medidor do lote** no topo (é o que o operador acompanha) e, na linha,
   um **anel** no disco de status + um **preenchimento do fundo da própria linha** —
   que some quando termina, então lista concluída fica limpa, só com os ✓. */

/* O valor do status vira CLASSE CSS (`"up-" + status`) — mexer aqui exige mexer
   nas regras `.up-queued/.up-uploading/.up-ok/.up-error` do globals.css. */
type UploadStatus = "queued" | "uploading" | "ok" | "error";
type Item = {
  id: string;
  file: File;
  status: UploadStatus;
  frac: number;
  error?: string;
};

const CONCURRENCY = 2;
const EXT_OK = /\.(mp3|flac|wav|ogg|m4a|aac|aiff?|opus)$/i;

export function UploadModal({
  categories,
  folders,
  onCreateFolder,
  onDone,
  onClose,
}: {
  categories: CategoryDTO[];
  /** pastas existentes; as `auto` (Spotify/YouTube/Sem pasta) não são atribuíveis */
  folders: FolderDTO[];
  onCreateFolder: (name: string) => Promise<void>;
  /** chamado quando o lote termina com pelo menos 1 sucesso (recarrega a biblioteca) */
  onDone: () => void;
  onClose: () => void;
}) {
  const [pickedFolders, setPickedFolders] = useState<string[]>([]);
  const [newFolder, setNewFolder] = useState("");
  const [items, setItems] = useState<Item[]>([]);
  const [category, setCategory] = useState(categories[0]?.key ?? "music");
  const [artist, setArtist] = useState("");
  const [album, setAlbum] = useState("");
  const [drag, setDrag] = useState(false);
  const [running, setRunning] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const seq = useRef(0);

  const add = useCallback((fs: FileList | File[] | null) => {
    if (!fs) return;
    const fresh: Item[] = [];
    for (const f of Array.from(fs)) {
      // aceita pelo tipo OU pela extensão: arquivo vindo de rede/pendrive às vezes
      // chega com type vazio, e recusá-lo confundiria mais do que ajudaria.
      if (!f.type.startsWith("audio/") && !EXT_OK.test(f.name)) continue;
      fresh.push({ id: `f${++seq.current}`, file: f, status: "queued", frac: 0 });
    }
    if (fresh.length === 0) return;
    setItems((prev) => {
      // dedupe por nome+tamanho: arrastar a mesma pasta duas vezes é comum
      const seen = new Set(prev.map((i) => i.file.name + ":" + i.file.size));
      return [...prev, ...fresh.filter((n) => !seen.has(n.file.name + ":" + n.file.size))];
    });
  }, []);

  const patch = (id: string, p: Partial<Item>) =>
    setItems((prev) => prev.map((i) => (i.id === id ? { ...i, ...p } : i)));

  const uploadOne = useCallback(
    async (it: Item) => {
      patch(it.id, { status: "uploading", frac: 0, error: undefined });
      try {
        await uploadTrackProgress(
          it.file,
          { category, title: "", artist, album, folders: pickedFolders },
          (frac) => patch(it.id, { frac }),
        );
        patch(it.id, { status: "ok", frac: 1 });
        return true;
      } catch (e) {
        patch(it.id, {
          status: "error",
          error: e instanceof Error ? e.message : "falha no envio",
        });
        return false;
      }
    },
    [category, artist, album, pickedFolders],
  );

  const uploadAll = useCallback(async () => {
    const pendingItems = items.filter((i) => i.status === "queued" || i.status === "error");
    if (pendingItems.length === 0 || running) return;
    setRunning(true);

    let ok = 0;
    let cursor = 0;
    // pool simples: N trabalhadores puxando da mesma fila
    const worker = async () => {
      for (;;) {
        const i = cursor++;
        if (i >= pendingItems.length) return;
        if (await uploadOne(pendingItems[i])) ok++;
      }
    };
    await Promise.all(
      Array.from({ length: Math.min(CONCURRENCY, pendingItems.length) }, worker),
    );

    setRunning(false);
    if (ok > 0) {
      onDone();
      toast.success(ok === 1 ? "1 faixa adicionada." : `${ok} faixas adicionadas.`);
    }
    const failed = pendingItems.length - ok;
    if (failed > 0) {
      toast.error(
        failed === 1
          ? "1 arquivo não foi enviado — veja o motivo na lista."
          : `${failed} arquivos não foram enviados — veja os motivos na lista.`,
      );
    }
  }, [items, running, uploadOne, onDone]);

  const summary = useMemo(() => {
    // as chaves são os PRÓPRIOS valores de UploadStatus (indexadas por i.status)
    const c = { queued: 0, uploading: 0, ok: 0, error: 0 };
    for (const i of items) c[i.status]++;
    return c;
  }, [items]);

  // A lista SEGUE o envio: com 80 arquivos o item em curso sai da janela em segundos
  // e o operador fica olhando linhas paradas. Só o PRIMEIRO em voo é rastreado
  // (concorrência 2 faria a lista pular entre dois) e o scroll é `nearest` — move o
  // mínimo, nunca recentraliza a lista inteira a cada arquivo.
  const activeId = items.find((i) => i.status === "uploading")?.id;
  const activeRef = useRef<HTMLLIElement>(null);
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeId]);

  const pending = summary.queued + summary.error;
  const totalProgress = items.length
    ? items.reduce((s, i) => s + (i.status === "ok" ? 1 : i.frac), 0) / items.length
    : 0;

  return (
    <div className="lib-overlay" onMouseDown={running ? undefined : onClose}>
      <div className="lib-modal up-modal" onMouseDown={(e) => e.stopPropagation()}>
        <header className="lib-titlebar">
          AutoDJ — Adicionar Mídia
          {items.length > 0 && <span className="up-count">{items.length} arquivo(s)</span>}
        </header>

        <div className="lib-body">
          <div
            className={"lib-drop up-drop" + (drag ? " over" : "")}
            onClick={() => inputRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault();
              setDrag(true);
            }}
            onDragLeave={() => setDrag(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDrag(false);
              add(e.dataTransfer.files);
            }}
          >
            <span className="up-drop-ico" aria-hidden="true">
              ♫
            </span>
            <span className="lib-drop-hint">
              Arraste os arquivos aqui ou clique para escolher
              <em> — pode selecionar vários de uma vez</em>
            </span>
            <input
              ref={inputRef}
              type="file"
              multiple
              accept="audio/*,.mp3,.flac,.wav,.ogg,.m4a,.aac,.aiff,.opus"
              hidden
              onChange={(e) => {
                add(e.target.files);
                e.target.value = ""; // permite re-escolher o mesmo arquivo
              }}
            />
          </div>

          {items.length > 0 && (
            <div className="up-batch">
              {/* Medidor do LOTE: é o número que o operador acompanha com 80 arquivos.
                  As linhas viram detalhe de consulta, não o painel principal. */}
              <div className="up-summary">
                <div
                  className="up-gauge"
                  role="progressbar"
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(totalProgress * 100)}
                  aria-label="Progresso do envio"
                >
                  <span
                    className={"up-gauge-fill" + (running ? " run" : "")}
                    style={{ width: `${totalProgress * 100}%` }}
                  />
                </div>
                <div className="up-pills">
                  <span className="up-pct">{Math.round(totalProgress * 100)}%</span>
                  {summary.uploading > 0 && (
                    <span className="up-pill up-pill-run">{summary.uploading} enviando</span>
                  )}
                  {summary.queued > 0 && <span className="up-pill">{summary.queued} na fila</span>}
                  {summary.ok > 0 && <span className="up-pill up-pill-ok">{summary.ok} ✓</span>}
                  {summary.error > 0 && (
                    <span className="up-pill up-pill-error">{summary.error} com erro</span>
                  )}
                  {!running && summary.ok > 0 && (
                    <button
                      className="up-clear"
                      onClick={() => setItems((p) => p.filter((x) => x.status !== "ok"))}
                    >
                      limpar enviadas
                    </button>
                  )}
                </div>
              </div>

              <ul className="up-list">
                {items.map((i) => (
                  <li
                    key={i.id}
                    className={"up-item up-" + i.status}
                    ref={i.id === activeId ? activeRef : undefined}
                  >
                    {/* fundo que enche: mesmo lugar do olhar que o nome do arquivo, e
                        some ao terminar (linha concluída não deixa rastro colorido) */}
                    <span
                      className="up-fill"
                      style={{ width: i.status === "uploading" ? `${i.frac * 100}%` : undefined }}
                      aria-hidden="true"
                    />
                    <span
                      className="up-disc"
                      style={{ ["--p" as string]: i.frac }}
                      aria-hidden="true"
                    >
                      <span className="up-disc-g">
                        {i.status === "ok" ? "✓" : i.status === "error" ? "!" : "♪"}
                      </span>
                    </span>
                    <span className="up-name" title={i.file.name}>
                      {i.file.name}
                    </span>
                    <span className="up-info">
                      {i.status === "uploading"
                        ? Math.round(i.frac * 100) + "%"
                        : i.status === "error"
                          ? i.error
                          : i.status === "ok"
                            ? "enviado"
                            : mb(i.file.size)}
                    </span>
                    {!running && i.status !== "ok" && (
                      <button
                        className="up-rm"
                        title="Remover da lista"
                        onClick={() => setItems((p) => p.filter((x) => x.id !== i.id))}
                      >
                        ✕
                      </button>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          )}

          <div className="up-meta">
            <label className="lib-field">
              <span>Categoria</span>
              <select
                value={category}
                onChange={(e) => setCategory(e.target.value)}
                disabled={running}
              >
                {categories.map((c) => (
                  <option key={c.key} value={c.key}>
                    {c.ico} {c.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="lib-field">
              <span>Intérprete</span>
              <input
                value={artist}
                onChange={(e) => setArtist(e.target.value)}
                placeholder="(opcional — vale para todos)"
                disabled={running}
              />
            </label>
            <label className="lib-field">
              <span>Álbum</span>
              <input
                value={album}
                onChange={(e) => setAlbum(e.target.value)}
                placeholder="(opcional — vale para todos)"
                disabled={running}
              />
            </label>
          </div>
          {/* Pastas do LOTE: valem pra todos os arquivos do envio. Só as do
              operador aparecem — Spotify/YouTube/"Sem pasta" são automáticas e não
              se atribui faixa a elas. */}
          <div className="up-folders">
            <span className="up-folders-label">Pastas</span>
            <div className="up-chips">
              {folders
                .filter((f) => !f.auto)
                .map((f) => {
                  const on = pickedFolders.some((p) => p.toLowerCase() === f.name.toLowerCase());
                  return (
                    <button
                      key={f.name}
                      type="button"
                      className={"up-chip" + (on ? " on" : "")}
                      disabled={running}
                      onClick={() =>
                        setPickedFolders((prev) =>
                          on
                            ? prev.filter((p) => p.toLowerCase() !== f.name.toLowerCase())
                            : [...prev, f.name],
                        )
                      }
                    >
                      {on ? "✓ " : ""}
                      {f.name}
                    </button>
                  );
                })}
              <input
                className="up-chip-new"
                value={newFolder}
                onChange={(e) => setNewFolder(e.target.value)}
                onKeyDown={async (e) => {
                  if (e.key !== "Enter") return;
                  const n = newFolder.trim();
                  if (!n) return;
                  await onCreateFolder(n); // cria e já deixa marcada
                  setPickedFolders((p) => [...p, n]);
                  setNewFolder("");
                }}
                placeholder="+ nova pasta (Enter)"
                maxLength={40}
                disabled={running}
              />
            </div>
          </div>

          <p className="up-nota">
            O título de cada faixa vem do próprio arquivo. Dá para ajustar depois pela
            edição.
          </p>
        </div>

        <footer className="lib-footer up-footer">
          {items.length > 0 && (
            <span className="up-status">
              {running
                ? `Enviando… ${Math.round(totalProgress * 100)}%`
                : summary.ok > 0
                  ? `${summary.ok} enviada(s)${summary.error ? ` · ${summary.error} com erro` : ""}`
                  : `${items.length} pronto(s) para enviar`}
            </span>
          )}
          <button className="lib-save" disabled={pending === 0 || running} onClick={uploadAll}>
            {running
              ? "Enviando…"
              : summary.error > 0 && summary.queued === 0
                ? "Tentar de novo ✔"
                : `Adicionar ${pending || ""} ✔`}
          </button>
          <button className="lib-close" onClick={onClose} disabled={running}>
            {summary.ok > 0 && pending === 0 ? "Fechar" : "Cancelar ⤴"}
          </button>
        </footer>
      </div>
    </div>
  );
}

function mb(bytes: number): string {
  return bytes >= 1024 * 1024
    ? (bytes / 1024 / 1024).toFixed(1) + " MB"
    : Math.round(bytes / 1024) + " KB";
}
