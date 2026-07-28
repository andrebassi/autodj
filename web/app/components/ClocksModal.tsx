"use client";

import { useEffect, useState } from "react";
import { useClocks } from "@/hooks/useClocks";
import { ClockPreview } from "@/components/ClockPreview";
import { type ClockDTO, type SlotDTO, type CategoryDTO } from "@/lib/api";
import { confirmDialog } from "@/lib/confirm";
import { toast } from "@/lib/toast";

/* clock slot kinds (mirrors domain/schedule.SlotKind) with pt-BR labels */
const SLOT_KINDS: { key: string; label: string }[] = [
  { key: "music", label: "Música" },
  { key: "jingle", label: "Vinheta" },
  { key: "commercial", label: "Comercial" },
  { key: "event", label: "Evento" },
];

/* slugify a name into a stable clock id (ascii, lowercase, dash-joined) */
function slugify(name: string): string {
  return name
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function newSlot(): SlotDTO {
  return { kind: "music", category: "music", count: 1 };
}

/* one editable slot row: kind + category + count */
function SlotRow({
  slot,
  categories,
  onChange,
  onRemove,
}: {
  slot: SlotDTO;
  categories: CategoryDTO[];
  onChange: (s: SlotDTO) => void;
  onRemove: () => void;
}) {
  return (
    <div className="mix-row">
      <select
        className="mix-num"
        value={slot.kind}
        onChange={(e) => onChange({ ...slot, kind: e.target.value })}
      >
        {SLOT_KINDS.map((k) => (
          <option key={k.key} value={k.key}>
            {k.label}
          </option>
        ))}
      </select>
      <select
        className="mix-num"
        value={slot.category}
        onChange={(e) => onChange({ ...slot, category: e.target.value })}
      >
        {categories.map((c) => (
          <option key={c.key} value={c.key}>
            {c.label}
          </option>
        ))}
      </select>
      <input
        className="mix-num sm"
        type="number"
        min={1}
        max={60}
        value={slot.count}
        onChange={(e) => onChange({ ...slot, count: +e.target.value })}
      />
      <button className="mix-close" onClick={onRemove}>
        ✕
      </button>
    </div>
  );
}

/* Programação — Relógios / Modelos de Hora (Story 4.1).
   Lista os clocks, edita nome + slots (tipo/categoria/quantidade), cria e
   exclui. Persiste via PUT/DELETE /api/v1/clocks. */
export function ClocksModal({
  categories,
  onClose,
}: {
  categories: CategoryDTO[];
  onClose: () => void;
}) {
  const { clocks, saving, error, save, remove } = useClocks();
  const [selId, setSelId] = useState<string | null>(null);
  const [draft, setDraft] = useState<ClockDTO | null>(null);
  const [preview, setPreview] = useState(false);

  // adopt the first clock as the editing draft once the list loads.
  useEffect(() => {
    if (draft || clocks.length === 0) return;
    setSelId(clocks[0].id);
    setDraft(structuredClone(clocks[0]));
  }, [clocks, draft]);

  const pick = (c: ClockDTO) => {
    setSelId(c.id);
    setDraft(structuredClone(c));
  };

  const startNew = () => {
    setSelId(null);
    setDraft({ id: "", name: "", slots: [newSlot()] });
  };

  const setSlot = (i: number, s: SlotDTO) =>
    setDraft((d) =>
      d ? { ...d, slots: d.slots.map((x, j) => (j === i ? s : x)) } : d,
    );
  const addSlot = () =>
    setDraft((d) => (d ? { ...d, slots: [...d.slots, newSlot()] } : d));
  const removeSlot = (i: number) =>
    setDraft((d) => (d ? { ...d, slots: d.slots.filter((_, j) => j !== i) } : d));

  const itemCount = draft ? draft.slots.reduce((n, s) => n + s.count, 0) : 0;

  const onSave = async () => {
    if (!draft) return;
    const id = draft.id || slugify(draft.name);
    if (!id) return;
    const ok = await save({ ...draft, id });
    if (ok) {
      setSelId(id);
      setDraft({ ...draft, id });
    }
  };

  const onDelete = async () => {
    if (!selId) return;
    const ok = await confirmDialog({
      title: `Excluir o relógio "${draft?.name}"?`,
      okText: "Excluir",
      danger: true,
    });
    if (!ok) return;
    if (await remove(selId)) {
      setSelId(null);
      setDraft(null);
      toast.success("Relógio excluído.");
    }
  };

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">
          AutoDJ — Relógios / Modelos de Hora
        </header>

        <div className="mix-body clocks-body">
          <aside className="clocks-list">
            {clocks.map((c) => (
              <button
                key={c.id}
                className={"clocks-item" + (c.id === selId ? " active" : "")}
                onClick={() => pick(c)}
              >
                {c.name || c.id}
              </button>
            ))}
            <button className="clocks-item new" onClick={startNew}>
              ＋ Novo relógio
            </button>
          </aside>

          <section className="clocks-edit">
            {draft ? (
              <>
                <div className="mix-inline">
                  <span className="mix-lbl">Nome</span>
                  <input
                    className="mix-num wide"
                    type="text"
                    value={draft.name}
                    onChange={(e) =>
                      setDraft((d) => (d ? { ...d, name: e.target.value } : d))
                    }
                  />
                </div>

                <fieldset className="mix-group">
                  <legend>Slots — {itemCount} itens/hora</legend>
                  {draft.slots.map((s, i) => (
                    <SlotRow
                      key={i}
                      slot={s}
                      categories={categories}
                      onChange={(ns) => setSlot(i, ns)}
                      onRemove={() => removeSlot(i)}
                    />
                  ))}
                  <button className="mix-close" onClick={addSlot}>
                    ＋ Slot
                  </button>
                </fieldset>
              </>
            ) : (
              <p className="clocks-empty">
                Selecione um relógio ou crie um novo.
              </p>
            )}
          </section>
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          {selId && (
            <button
              className="mix-close"
              onClick={() => setPreview(true)}
              disabled={saving}
            >
              Prever hora 👁
            </button>
          )}
          {selId && (
            <button className="mix-close" onClick={onDelete} disabled={saving}>
              Excluir 🗑
            </button>
          )}
          <button
            className="mix-save"
            onClick={onSave}
            disabled={saving || !draft || (!draft.name && !draft.id)}
          >
            {saving ? "Salvando…" : "Salvar ✔"}
          </button>
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>

      {preview && selId && (
        <ClockPreview
          clockId={selId}
          clockName={draft?.name ?? selId}
          onClose={() => setPreview(false)}
        />
      )}
    </div>
  );
}
