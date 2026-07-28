import { MONTHS, WEEKDAYS } from "@/lib/format";

/* big HH:MM:SS clock + date (header center).
   steady = separadores param de piscar (quando algo está tocando). */
export function ClockBlock({ now, steady = false }: { now: Date | null; steady?: boolean }) {
  const hh = now ? String(now.getHours()).padStart(2, "0") : "--";
  const mm = now ? String(now.getMinutes()).padStart(2, "0") : "--";
  const ss = now ? String(now.getSeconds()).padStart(2, "0") : "--";
  const dateDay = now ? now.getDate() : "";
  const dateMonth = now ? MONTHS[now.getMonth()] : "";
  const dateWeek = now ? WEEKDAYS[now.getDay()] : "";

  return (
    <div className="clock-block">
      <div className={"clock" + (steady ? " steady" : "")}>
        {hh}
        <b>:</b>
        {mm}
        <b>:</b>
        {ss}
      </div>
      <div className="date">
        <span className="d-day">{dateDay}</span> {dateMonth}
        <br />
        <span className="d-week">{dateWeek}</span>
      </div>
    </div>
  );
}
