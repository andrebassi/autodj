import { classify, fmtDur, KIND_ICON } from "@/lib/format";
import type { PlayedItem } from "@/lib/types";
import { playableToMedia } from "@/lib/operational";
import { TrackCover } from "@/components/TrackCover";
import { SourceBadge } from "@/components/SourceBadge";

/* data+hora local em que a faixa saiu do ar (rastreabilidade). Vazio se sem carimbo. */
function whenLabel(ms?: number): string {
  if (!ms) return "";
  const d = new Date(ms);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getDate())}/${p(d.getMonth() + 1)} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

/* Uma faixa já tocada (aba Tocadas). VISUAL IDÊNTICO à QueueRow (fila) — "como se
   tivesse movido pra lá": mesmas colunas (.row.q → mesmo grid/estilo), mesma pílula
   de posição, mesma fonte/origem. Diferenças de histórico: não é arrastável (grip só
   preenche a coluna, sem affordance), a coluna transição mostra "tocada", e o carimbo
   de data/hora vira tooltip da linha (a Fila não tem coluna de hora). */
export function HistoryRow({
  item,
  pos,
  plays,
  onOpenDetail,
}: {
  item: PlayedItem;
  pos?: number; // posição na sequência de execução (tocada nº) — espelha QueueRow
  plays?: number; // total de vezes que ESTA faixa tocou na sessão (badge só se >1)
  onOpenDetail?: (media: ReturnType<typeof playableToMedia>) => void;
}) {
  const kind = classify(item.title);
  const origin = item.origin ?? "";
  const when = whenLabel(item.played_at);
  const repeated = (plays ?? 0) > 1;
  return (
    <li className={`row q k-${kind}`} title={when ? `Tocada ${when}` : undefined}>
      <span className="r-pos" aria-label={pos ? `Posição ${pos}` : undefined}>
        {pos ?? ""}
      </span>
      <span className="r-grip" aria-hidden="true" />
      <span className="r-ico">{KIND_ICON[kind]}</span>
      <span className="r-check">✓</span>
      <TrackCover
        trackId={item.track_id}
        src={item.artwork_url}
        size="row"
        alt={item.title}
        onClick={
          onOpenDetail ? () => onOpenDetail(playableToMedia(item)) : undefined
        }
      />
      <span className="r-title">
        <span className="r-name">{item.title || item.track_id}</span>
        {repeated && (
          <span
            className="r-repeat"
            title={`Tocou ${plays}× nesta sessão (repetida)`}
          >
            🔁 {plays}×
          </span>
        )}
        {item.artist && <span className="r-artist">{item.artist}</span>}
      </span>
      <span className="r-src">
        <SourceBadge source={item.source} />
      </span>
      <span className="r-origin" title={origin ? `De: ${origin}` : undefined}>
        {origin || "—"}
      </span>
      <span className="r-trans">tocada</span>
      <span className="r-dur">{fmtDur(item.duration_ms)}</span>
    </li>
  );
}
