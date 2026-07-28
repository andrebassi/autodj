"use client";

import { useEffect, useMemo, useState } from "react";
import { useGrid } from "@/hooks/useGrid";
import { useClocks } from "@/hooks/useClocks";
import { type GridDTO } from "@/lib/api";

/* weekday labels, index = time.Weekday (0=Sunday..6=Saturday) */
const WEEKDAYS = ["Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"];
const HOURS = Array.from({ length: 24 }, (_, h) => h);

/* stable per-clock swatch colour so a painted cell is recognizable at a glance.
   Empty cells get no colour. Derived from the clock id hash → hue. */
function hue(id: string): number {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) % 360;
  return h;
}

/* Programação — Grade Semanal (Story 4.2).
   7×24 matrix (weekday × hour) mapping each hour to a clock. Pick an active
   clock in the palette, then click cells (or a weekday label to paint the whole
   row) to assign it. "(vazio)" clears. Persists the whole grid via PUT /grid. */
export function GradeModal({ onClose }: { onClose: () => void }) {
  const { grid, saving, error, save } = useGrid();
  const { clocks } = useClocks();
  const [cells, setCells] = useState<Map<string, string>>(new Map());
  const [active, setActive] = useState<string>("");

  const key = (wd: number, h: number) => `${wd}-${h}`;

  // adopt the loaded grid into the editable map once, and default the paint
  // palette to the first clock.
  useEffect(() => {
    const m = new Map<string, string>();
    for (const c of grid.cells) m.set(key(c.weekday, c.hour), c.clockId);
    setCells(m);
  }, [grid]);
  useEffect(() => {
    if (!active && clocks.length > 0) setActive(clocks[0].id);
  }, [clocks, active]);

  const name = useMemo(() => {
    const byID = new Map(clocks.map((c) => [c.id, c.name || c.id]));
    return (id: string) => byID.get(id) ?? id;
  }, [clocks]);

  const paint = (wd: number, h: number) =>
    setCells((prev) => {
      const next = new Map(prev);
      if (active === "") next.delete(key(wd, h));
      else next.set(key(wd, h), active);
      return next;
    });

  const paintRow = (wd: number) =>
    setCells((prev) => {
      const next = new Map(prev);
      for (const h of HOURS) {
        if (active === "") next.delete(key(wd, h));
        else next.set(key(wd, h), active);
      }
      return next;
    });

  const onSave = async () => {
    const payload: GridDTO = {
      cells: [...cells.entries()].map(([k, clockId]) => {
        const [wd, h] = k.split("-").map(Number);
        return { weekday: wd, hour: h, clockId };
      }),
    };
    await save(payload);
  };

  const total = cells.size;

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Grade Semanal</header>

        <div className="mix-body grade-body">
          <div className="mix-inline grade-palette">
            <span className="mix-lbl">Relógio ativo</span>
            <select
              className="mix-num wide"
              value={active}
              onChange={(e) => setActive(e.target.value)}
            >
              {clocks.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name || c.id}
                </option>
              ))}
              <option value="">(vazio — limpar)</option>
            </select>
            <span className="grade-hint">
              {total} de 168 horas programadas. Clique numa célula, ou no dia para
              pintar a linha inteira.
            </span>
          </div>

          <div className="grade-scroll">
            <table className="grade-table">
              <thead>
                <tr>
                  <th className="grade-corner"></th>
                  {HOURS.map((h) => (
                    <th key={h} className="grade-hour">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {WEEKDAYS.map((wdLabel, wd) => (
                  <tr key={wd}>
                    <th className="grade-day" onClick={() => paintRow(wd)}>
                      {wdLabel}
                    </th>
                    {HOURS.map((h) => {
                      const id = cells.get(key(wd, h));
                      return (
                        <td
                          key={h}
                          className={"grade-cell" + (id ? " on" : "")}
                          title={id ? name(id) : "vazio"}
                          style={
                            id
                              ? { background: `hsl(${hue(id)} 55% 42%)` }
                              : undefined
                          }
                          onClick={() => paint(wd, h)}
                        >
                          {id ? name(id).slice(0, 2) : ""}
                        </td>
                      );
                    })}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-save" onClick={onSave} disabled={saving}>
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
