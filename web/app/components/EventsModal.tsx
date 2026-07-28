"use client";

import { useEffect, useState } from "react";
import { useEvents } from "@/hooks/useEvents";
import { EVENT_KINDS, type ScheduledEventDTO, type CategoryDTO } from "@/lib/api";
import { confirmDialog } from "@/lib/confirm";
import { toast } from "@/lib/toast";

/* event kinds (mirrors domain/schedule.EventKind) with pt-BR labels */
const KIND_LABELS: Record<string, string> = {
  voz_do_brasil: "Voz do Brasil",
  hora_certa: "Hora certa",
  temperatura: "Temperatura",
  comercial: "Comercial",
  programete: "Programete",
};

/* weekdays 0=Sun..6=Sat (matches time.Weekday) */
const WEEKDAYS: { n: number; label: string }[] = [
  { n: 0, label: "Dom" },
  { n: 1, label: "Seg" },
  { n: 2, label: "Ter" },
  { n: 3, label: "Qua" },
  { n: 4, label: "Qui" },
  { n: 5, label: "Sex" },
  { n: 6, label: "Sáb" },
];

/* slugify a name into a stable event id (ascii, lowercase, dash-joined) */
function slugify(name: string): string {
  return name
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

const pad2 = (n: number) => String(n).padStart(2, "0");

function newEvent(): ScheduledEventDTO {
  return {
    id: "",
    kind: "comercial",
    days: [1, 2, 3, 4, 5],
    hour: 8,
    minute: 0,
    priority: 50,
    mediaRef: "",
  };
}

/* Programação — Eventos agendados (Story 4.4). Lista os eventos, edita tipo /
   dias / hora:minuto / prioridade / mídia, cria e exclui. A mídia (mediaRef)
   resolve por id de faixa exato, senão por categoria; vazio = gerado depois
   (ex. TTS hora certa). Persiste via PUT/DELETE /api/v1/events. */
export function EventsModal({
  categories,
  onClose,
}: {
  categories: CategoryDTO[];
  onClose: () => void;
}) {
  const { events, saving, error, save, remove } = useEvents();
  const [selId, setSelId] = useState<string | null>(null);
  const [draft, setDraft] = useState<ScheduledEventDTO | null>(null);

  // adopt the first event as the editing draft once the list loads.
  useEffect(() => {
    if (draft || events.length === 0) return;
    setSelId(events[0].id);
    setDraft(structuredClone(events[0]));
  }, [events, draft]);

  const pick = (e: ScheduledEventDTO) => {
    setSelId(e.id);
    setDraft(structuredClone(e));
  };

  const startNew = () => {
    setSelId(null);
    setDraft(newEvent());
  };

  const toggleDay = (n: number) =>
    setDraft((d) => {
      if (!d) return d;
      const has = d.days.includes(n);
      const days = has ? d.days.filter((x) => x !== n) : [...d.days, n].sort((a, b) => a - b);
      return { ...d, days };
    });

  const onSave = async () => {
    if (!draft) return;
    const id = draft.id || slugify(KIND_LABELS[draft.kind] + "-" + pad2(draft.hour) + pad2(draft.minute));
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
      title: `Excluir o evento "${draft?.id}"?`,
      okText: "Excluir",
      danger: true,
    });
    if (!ok) return;
    if (await remove(selId)) {
      setSelId(null);
      setDraft(null);
      toast.success("Evento excluído.");
    }
  };

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Eventos agendados</header>

        <div className="mix-body clocks-body">
          <aside className="clocks-list">
            {events.map((e) => (
              <button
                key={e.id}
                className={"clocks-item" + (e.id === selId ? " active" : "")}
                onClick={() => pick(e)}
              >
                {pad2(e.hour)}:{pad2(e.minute)} · {KIND_LABELS[e.kind] ?? e.kind}
              </button>
            ))}
            <button className="clocks-item new" onClick={startNew}>
              ＋ Novo evento
            </button>
          </aside>

          <section className="clocks-edit">
            {draft ? (
              <>
                <div className="mix-inline">
                  <span className="mix-lbl">Tipo</span>
                  <select
                    className="mix-num wide"
                    value={draft.kind}
                    onChange={(e) =>
                      setDraft((d) => (d ? { ...d, kind: e.target.value } : d))
                    }
                  >
                    {EVENT_KINDS.map((k) => (
                      <option key={k} value={k}>
                        {KIND_LABELS[k] ?? k}
                      </option>
                    ))}
                  </select>
                </div>

                <fieldset className="mix-group">
                  <legend>Dias</legend>
                  <div className="ev-days">
                    {WEEKDAYS.map((w) => (
                      <button
                        key={w.n}
                        className={"ev-day" + (draft.days.includes(w.n) ? " active" : "")}
                        onClick={() => toggleDay(w.n)}
                      >
                        {w.label}
                      </button>
                    ))}
                  </div>
                </fieldset>

                <div className="mix-inline">
                  <span className="mix-lbl">Hora</span>
                  <input
                    className="mix-num sm"
                    type="number"
                    min={0}
                    max={23}
                    value={draft.hour}
                    onChange={(e) =>
                      setDraft((d) => (d ? { ...d, hour: +e.target.value } : d))
                    }
                  />
                  <span className="mix-lbl">Min</span>
                  <input
                    className="mix-num sm"
                    type="number"
                    min={0}
                    max={59}
                    value={draft.minute}
                    onChange={(e) =>
                      setDraft((d) => (d ? { ...d, minute: +e.target.value } : d))
                    }
                  />
                </div>

                <div className="mix-inline">
                  <span className="mix-lbl">Prioridade</span>
                  <input
                    className="mix-num sm"
                    type="number"
                    min={0}
                    max={100}
                    value={draft.priority}
                    onChange={(e) =>
                      setDraft((d) => (d ? { ...d, priority: +e.target.value } : d))
                    }
                  />
                  <span className="mix-hint">maior preempta</span>
                </div>

                <div className="mix-inline">
                  <span className="mix-lbl">Mídia</span>
                  <input
                    className="mix-num wide"
                    type="text"
                    list="ev-media-cats"
                    placeholder="categoria ou id de faixa (vazio = gerado)"
                    value={draft.mediaRef}
                    onChange={(e) =>
                      setDraft((d) => (d ? { ...d, mediaRef: e.target.value } : d))
                    }
                  />
                  <datalist id="ev-media-cats">
                    {categories.map((c) => (
                      <option key={c.key} value={c.key}>
                        {c.label}
                      </option>
                    ))}
                  </datalist>
                </div>
              </>
            ) : (
              <p className="clocks-empty">Selecione um evento ou crie um novo.</p>
            )}
          </section>
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          {selId && (
            <button className="mix-close" onClick={onDelete} disabled={saving}>
              Excluir 🗑
            </button>
          )}
          <button
            className="mix-save"
            onClick={onSave}
            disabled={saving || !draft || (draft?.days.length ?? 0) === 0}
          >
            {saving ? "Salvando…" : "Salvar ✔"}
          </button>
          <button className="mix-close" onClick={onClose}>
            Fechar ⤴
          </button>
        </footer>
      </div>
    </div>
  );
}
