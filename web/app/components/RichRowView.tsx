import { ROW_META, type RichRow } from "@/lib/mockProgram";

/* one program row — green (music/vinheta), lilac (commercial), or live-read box */
export function RichRowView({ r }: { r: RichRow }) {
  const m = ROW_META[r.t];
  if (r.t === "locucao") {
    return (
      <li className="rr rr-locucao">
        <span className="rr-ico">{m.ico}</span>
        <span className="rr-check">✓</span>
        <div className="loc-box">
          <div className="loc-head">** LOCUÇÃO AO VIVO **</div>
          <div className="loc-text">{r.text}</div>
        </div>
      </li>
    );
  }
  return (
    <li className={`rr ${m.cls}`}>
      <span className="rr-ico">{m.ico}</span>
      <span className="rr-check">✓</span>
      <span className="rr-label">{r.label}</span>
      <span className="rr-dur">{r.dur}</span>
    </li>
  );
}
