/* orange "LOCUÇÃO AO VIVO" now-playing row (mic + dual timer + wall clock) */
export function LiveAnnounceRow({
  up = "00:00",
  down = "00:00",
  wall,
}: {
  up?: string;
  down?: string;
  wall: string;
}) {
  return (
    <li className="pr pr-live">
      <span className="pr-ico">🎙</span>
      <div className="pr-body">
        <div className="pr-artist">** LOCUÇÃO AO VIVO **</div>
        <div className="pr-title">Ao Vivo</div>
      </div>
      <div className="pr-timers pr-timers-live">
        <span className="pr-up">
          {up} <span className="pr-tico up">⊕</span>
        </span>
        <span className="pr-wall">
          {wall} <span className="pr-tico">🕐</span>
        </span>
        <span className="pr-down">
          {down} <span className="pr-tico down">⊖</span>
        </span>
      </div>
    </li>
  );
}
