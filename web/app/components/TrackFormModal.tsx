"use client";

import { useState } from "react";
import type { CategoryDTO, FolderDTO, TrackForm } from "@/lib/api";
import type { OpMedia } from "@/lib/operational";
import { toast } from "@/lib/toast";

/* Editar mídia — metadados (título/intérprete/álbum/categoria) + PASTAS.
   Fonte/arquivo/duração nunca mudam (reclassificação sem re-upload).

   As pastas entram aqui porque só o envio sabia atribuí-las (v0.2.26): quem já
   tinha acervo dentro do app ficava com tudo preso em "Sem pasta", sem caminho
   nenhum pra organizar sem reenviar os arquivos.

   Faixa de streaming não mostra o bloco: a pasta dela é derivada da fonte
   (Spotify/YouTube) e o servidor recusa a troca — oferecer o controle seria
   prometer o que não acontece. */
export function TrackFormModal({
  media,
  categories,
  folders,
  onCreateFolder,
  onSave,
  onSaveFolders,
  onClose,
}: {
  media: OpMedia;
  categories: CategoryDTO[];
  folders: FolderDTO[];
  onCreateFolder: (name: string) => Promise<void>;
  onSave: (id: string, form: TrackForm) => Promise<void>;
  onSaveFolders: (id: string, folders: string[]) => Promise<void>;
  onClose: () => void;
}) {
  const [title, setTitle] = useState(media.title);
  const [artist, setArtist] = useState(media.artist === "—" ? "" : media.artist);
  const [album, setAlbum] = useState(media.album ?? "");
  const [category, setCategory] = useState(media.category ?? "music");
  const [pickedFolders, setPickedFolders] = useState<string[]>(media.folders ?? []);
  const [newFolder, setNewFolder] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const isStreaming = media.source === "spotify" || media.source === "youtube";
  const isPicked = (n: string) => pickedFolders.some((p) => p.toLowerCase() === n.toLowerCase());
  const toggleFolder = (n: string) =>
    setPickedFolders((prev) =>
      isPicked(n) ? prev.filter((p) => p.toLowerCase() !== n.toLowerCase()) : [...prev, n],
    );

  const submit = async () => {
    if (!media.id || busy) return;
    if (!title.trim()) {
      setErr("Título é obrigatório.");
      return;
    }
    setBusy(true);
    setErr(null);
    try {
      await onSave(media.id, { title, artist, album, category });
      // pastas vão por rota própria (PUT .../folders) — o update de metadados não
      // as carrega. Só chama se mudou, pra não gravar à toa a cada edição de título.
      const before = [...(media.folders ?? [])].sort().join("|").toLowerCase();
      const after = [...pickedFolders].sort().join("|").toLowerCase();
      if (!isStreaming && before !== after) await onSaveFolders(media.id, pickedFolders);
      toast.success(`"${title}" salva.`);
      onClose();
    } catch {
      setErr("Falha ao salvar. Tente de novo.");
      setBusy(false);
    }
  };

  return (
    <div className="lib-overlay" onMouseDown={onClose}>
      <div className="lib-modal" onMouseDown={(e) => e.stopPropagation()}>
        <header className="lib-titlebar">AutoDJ — Editar Mídia</header>

        <div className="lib-body">
          <label className="lib-field">
            <span>Título</span>
            <input value={title} onChange={(e) => setTitle(e.target.value)} />
          </label>
          <label className="lib-field">
            <span>Intérprete</span>
            <input value={artist} onChange={(e) => setArtist(e.target.value)} />
          </label>
          <label className="lib-field">
            <span>Álbum</span>
            <input value={album} onChange={(e) => setAlbum(e.target.value)} />
          </label>
          <label className="lib-field">
            <span>Categoria</span>
            <select value={category} onChange={(e) => setCategory(e.target.value)}>
              {categories.map((c) => (
                <option key={c.key} value={c.key}>
                  {c.ico} {c.label}
                </option>
              ))}
            </select>
          </label>

          {!isStreaming && (
            <div className="up-folders">
              <span className="up-folders-label">Pastas</span>
              <div className="up-chips">
                {folders
                  .filter((f) => !f.auto)
                  .map((f) => (
                    <button
                      key={f.name}
                      type="button"
                      className={"up-chip" + (isPicked(f.name) ? " on" : "")}
                      disabled={busy}
                      onClick={() => toggleFolder(f.name)}
                    >
                      {isPicked(f.name) ? "✓ " : ""}
                      {f.name}
                    </button>
                  ))}
                <input
                  className="up-chip-new"
                  value={newFolder}
                  onChange={(e) => setNewFolder(e.target.value)}
                  onKeyDown={async (e) => {
                    if (e.key !== "Enter") return;
                    e.preventDefault(); // Enter aqui cria pasta, não submete o formulário
                    const n = newFolder.trim();
                    if (!n) return;
                    await onCreateFolder(n);
                    setPickedFolders((p) => [...p, n]); // criada já entra marcada
                    setNewFolder("");
                  }}
                  placeholder="+ nova pasta (Enter)"
                  maxLength={40}
                  disabled={busy}
                />
              </div>
            </div>
          )}

          {err && <div className="lib-err">{err}</div>}
        </div>

        <footer className="lib-footer">
          <button className="lib-save" disabled={busy} onClick={submit}>
            {busy ? "Salvando…" : "Salvar ✔"}
          </button>
          <button className="lib-close" onClick={onClose}>
            Cancelar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
