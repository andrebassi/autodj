import { Timers } from "@/components/Timers";
import { ClockBlock } from "@/components/ClockBlock";

/* header band: now-playing title/next + timers + clock */
export function NowPlayingHeader({
  title,
  next,
  hasNext,
  now,
  posMs = 0,
  durMs = 0,
  queueMs = 0,
  showTimers = true,
  clockSteady = false,
}: {
  title: string;
  next: string;
  hasNext: boolean;
  now: Date | null;
  posMs?: number;
  durMs?: number;
  queueMs?: number;
  showTimers?: boolean; // timers de estação (⏱/＋/－/RDS) só no modo Rádio
  clockSteady?: boolean; // separadores do relógio param de piscar quando tocando
}) {
  return (
    <section className="np">
      <div className="np-left">
        <div className="np-title">{title}</div>
        {hasNext && (
          <div className="np-next">
            <span className="np-next-tag">▸ PRÓXIMA</span>
            <span className="np-next-name">{next}</span>
          </div>
        )}
      </div>

      <div className="np-right">
        {showTimers && <Timers posMs={posMs} durMs={durMs} queueMs={queueMs} />}
        <ClockBlock now={now} steady={clockSteady} />
      </div>
    </section>
  );
}
