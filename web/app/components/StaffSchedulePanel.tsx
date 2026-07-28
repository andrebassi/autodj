"use client";

import { useState } from "react";
import { useStaffSchedule } from "@/hooks/useStaffSchedule";
import type { StaffShiftDTO } from "@/lib/api";

/* names of the week, index = weekday (0=Sun … 6=Sat). Local UI labels only. */
const WEEKDAYS = ["Domingo", "Segunda", "Terça", "Quarta", "Quinta", "Sexta", "Sábado"];

/* StaffSchedulePanel — the announcer roster (Story 9.1): the *human* grid of who
   is on air in each weekday time slot. Not audio, not the program grid — pure
   organizational metadata. Lists shifts grouped by weekday, an add form
   (locutor / dia / início / fim), removal, and highlights whoever is on air now.
   Times are HH:MM in the UI and minutes-since-midnight on the wire. */
export function StaffSchedulePanel({ onClose }: { onClose: () => void }) {
  const { shifts, onAir, loaded, busy, available, error, add, remove } = useStaffSchedule();
  const [name, setName] = useState("");
  const [weekday, setWeekday] = useState(1); // Monday
  const [start, setStart] = useState("08:00");
  const [end, setEnd] = useState("10:00");

  const onAirID = onAir?.on_air ? onAir.shift?.id : undefined;

  const submit = async () => {
    const s = hmToMin(start);
    const e = hmToMin(end);
    if (s === null || e === null) return;
    const ok = await add({
      announcer_id: "",
      announcer_name: name,
      weekday,
      start_min: s,
      end_min: e,
    });
    if (ok) setName("");
  };

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Escala de trabalho</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}
          {loaded && !available && (
            <p className="mix-lbl">Escala indisponível (endpoint não configurado).</p>
          )}
          {loaded && available && (
            <>
              <fieldset className="mix-group">
                <legend>No ar agora</legend>
                {onAir?.on_air && onAir.shift ? (
                  <p className="staff-onair">
                    🔴 <strong>{onAir.shift.announcer_name}</strong>{" "}
                    <span className="mono">
                      ({WEEKDAYS[onAir.shift.weekday]} {minToHM(onAir.shift.start_min)}–
                      {minToHM(onAir.shift.end_min)})
                    </span>
                  </p>
                ) : (
                  <p className="mix-lbl">Ninguém escalado neste horário.</p>
                )}
              </fieldset>

              <fieldset className="mix-group">
                <legend>Novo turno</legend>
                <div className="mix-row">
                  <label className="mix-lbl">Locutor</label>
                  <input
                    className="staff-input"
                    value={name}
                    placeholder="Nome do locutor"
                    onChange={(ev) => setName(ev.target.value)}
                  />
                </div>
                <div className="mix-row">
                  <label className="mix-lbl">Dia</label>
                  <select
                    className="staff-select"
                    value={weekday}
                    onChange={(ev) => setWeekday(Number(ev.target.value))}
                  >
                    {WEEKDAYS.map((d, i) => (
                      <option value={i} key={i}>
                        {d}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="mix-row">
                  <label className="mix-lbl">Início</label>
                  <input
                    className="staff-time"
                    type="time"
                    value={start}
                    onChange={(ev) => setStart(ev.target.value)}
                  />
                  <label className="mix-lbl">Fim</label>
                  <input
                    className="staff-time"
                    type="time"
                    value={end}
                    onChange={(ev) => setEnd(ev.target.value)}
                  />
                </div>
                <div className="mix-footer">
                  <button
                    className="mix-save"
                    onClick={() => void submit()}
                    disabled={busy || name.trim() === ""}
                  >
                    Adicionar turno
                  </button>
                </div>
              </fieldset>

              <fieldset className="mix-group">
                <legend>Grade semanal</legend>
                {shifts.length === 0 ? (
                  <p className="mix-lbl">Nenhum turno cadastrado.</p>
                ) : (
                  WEEKDAYS.map((day, wd) => {
                    const dayShifts = shifts.filter((s) => s.weekday === wd);
                    if (dayShifts.length === 0) return null;
                    return (
                      <div className="staff-day" key={wd}>
                        <span className="staff-dayname">{day}</span>
                        <ul className="staff-list">
                          {dayShifts.map((s: StaffShiftDTO) => (
                            <li
                              className={"staff-shift" + (s.id === onAirID ? " staff-live" : "")}
                              key={s.id}
                            >
                              <span className="staff-slot mono">
                                {minToHM(s.start_min)}–{minToHM(s.end_min)}
                              </span>
                              <span className="staff-who">{s.announcer_name}</span>
                              <button
                                className="staff-rm"
                                onClick={() => void remove(s.id)}
                                disabled={busy}
                                title="Remover turno"
                              >
                                ✕
                              </button>
                            </li>
                          ))}
                        </ul>
                      </div>
                    );
                  })
                )}
              </fieldset>
            </>
          )}
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-close" onClick={onClose}>
            Fechar
          </button>
        </footer>
      </div>
    </div>
  );
}

/* hmToMin converts "HH:MM" to minutes since midnight, or null when malformed. */
function hmToMin(hm: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(hm);
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  if (h < 0 || h > 23 || min < 0 || min > 59) return null;
  return h * 60 + min;
}

/* minToHM converts minutes since midnight to "HH:MM". */
function minToHM(total: number): string {
  const h = Math.floor(total / 60);
  const m = total % 60;
  return String(h).padStart(2, "0") + ":" + String(m).padStart(2, "0");
}
