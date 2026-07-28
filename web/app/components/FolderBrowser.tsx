"use client";

import { useState } from "react";
import type { FolderDTO } from "@/lib/api";
import { BrandIcon, isBrand } from "@/components/BrandIcon";

/* Navegador de pastas da biblioteca (Músicas → Pastas).

   Três tipos de pasta convivem, e a diferença importa pro operador:
   - **automáticas de fonte** (Spotify, YouTube): derivadas da origem da faixa.
     Não se cria, não se apaga, não se move faixa pra dentro nem pra fora.
   - **"Sem pasta"**: virtual, junta as locais que ainda não foram organizadas.
     Aparece de propósito (decisão do operador) pra ninguém perder faixa de vista.
   - **as suas**: criadas por você, renomeáveis e removíveis.

   Só as suas mostram o botão de excluir — oferecer "apagar" numa pasta automática
   seria prometer algo que o sistema não faz.

   Excluir NUNCA apaga faixa: elas voltam pra "Sem pasta". A confirmação diz isso,
   porque "excluir pasta" com música dentro assusta com razão. */
export function FolderBrowser({
  folders,
  selected,
  onPick,
  onCreate,
  onDelete,
}: {
  folders: FolderDTO[];
  selected: string;
  onPick: (name: string) => void;
  onCreate: (name: string) => Promise<void>;
  onDelete: (name: string) => Promise<void>;
}) {
  const [newName, setNewName] = useState("");
  const [creating, setCreating] = useState(false);

  const submitCreate = async () => {
    const n = newName.trim();
    if (!n || creating) return;
    setCreating(true);
    try {
      await onCreate(n);
      setNewName("");
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="fb">
      <div className="fb-new">
        <input
          className="fb-input"
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void submitCreate();
          }}
          placeholder="Nova pasta (ex.: rock, anos 90)"
          maxLength={40}
          disabled={creating}
        />
        <button className="fb-add" onClick={() => void submitCreate()} disabled={!newName.trim() || creating}>
          + Criar
        </button>
      </div>

      <div className="fb-grid">
        {/* "Todas" zera o filtro — sem isso o operador entra numa pasta e não
            encontra o caminho de volta pra biblioteca inteira. */}
        <button
          className={"fb-item" + (selected === "" ? " on" : "")}
          onClick={() => onPick("")}
        >
          <span className="fb-ico" aria-hidden="true">
            🗃
          </span>
          <span className="fb-name">Todas</span>
        </button>

        {folders.map((f) => (
          <button
            key={f.name}
            className={"fb-item" + (selected === f.name ? " on" : "") + (f.auto ? " auto" : "")}
            onClick={() => onPick(f.name)}
            title={`${f.count} ${f.count === 1 ? "faixa" : "faixas"}`}
          >
            <span className="fb-ico" aria-hidden="true">
              <FolderIcon folder={f} />
            </span>
            <span className="fb-name">{f.name}</span>
            <span className="fb-count">{f.count}</span>
            {!f.auto && (
              <span
                className="fb-del"
                role="button"
                tabIndex={0}
                title="Excluir pasta (as faixas continuam na biblioteca)"
                onClick={(e) => {
                  e.stopPropagation(); // não selecionar a pasta ao clicar no X
                  void onDelete(f.name);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.stopPropagation();
                    void onDelete(f.name);
                  }
                }}
              >
                ✕
              </span>
            )}
          </button>
        ))}
      </div>

      {folders.length === 0 && (
        <p className="fb-empty">
          Nenhuma pasta ainda. Crie uma acima e escolha-a ao enviar músicas.
        </p>
      )}
    </div>
  );
}

/* Ícone da pasta. As de serviço usam o LOGO real (mesmo componente das abas e dos
   painéis, v0.2.23) — emoji colorido não diz de qual serviço se trata e ainda muda
   de desenho por sistema operacional. As demais seguem com emoji porque não têm
   marca: "Sem pasta" é organizacional, e as suas são pastas comuns. */
function FolderIcon({ folder }: { folder: FolderDTO }) {
  const brand = folder.name.toLowerCase();
  if (isBrand(brand)) return <BrandIcon name={brand} size={14} decorative />;
  return <>{folder.auto ? "🗂" : "📁"}</>;
}
