"use client";

import { useCallback, useEffect, useState } from "react";
import { listFolders, createFolder, deleteFolder, type FolderDTO } from "@/lib/api";

/* Pastas do operador + as automáticas de fonte (Spotify/YouTube) e a virtual
   "Sem pasta". A lista vem PRONTA do servidor, com contagem — a UI não recalcula
   nada: se ela decidisse quem entra, a listagem e o filtro divergiriam na primeira
   regra nova (foi por isso que `TrackFolders` mora no domínio). */
export function useFolders(): {
  folders: FolderDTO[];
  loading: boolean;
  reload: () => void;
  create: (name: string) => Promise<void>;
  remove: (name: string) => Promise<void>;
} {
  const [folders, setFolders] = useState<FolderDTO[]>([]);
  const [loading, setLoading] = useState(true);

  const reload = useCallback(() => {
    setLoading(true);
    void listFolders()
      .then(setFolders)
      .catch(() => setFolders([]))
      .finally(() => setLoading(false));
  }, []);

  useEffect(reload, [reload]);

  const create = useCallback(
    async (name: string) => {
      await createFolder(name);
      reload();
    },
    [reload],
  );

  const remove = useCallback(
    async (name: string) => {
      await deleteFolder(name);
      reload();
    },
    [reload],
  );

  return { folders, loading, reload, create, remove };
}
