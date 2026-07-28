"use client";

/* Crossfader slider (0 = deck A full, 1 = deck B full). Drives the equal-power
   curve in useDeckPair; the center detent (0.5) is the ~0.707/0.707 blend. */
export function Crossfader({
  pos,
  onChange,
}: {
  pos: number;
  onChange: (pos: number) => void;
}) {
  return (
    <div className="xfader">
      <span className="xfader-tag">A</span>
      <input
        className="xfader-slider"
        type="range"
        min={0}
        max={1}
        step={0.01}
        value={pos}
        onChange={(e) => onChange(Number(e.target.value))}
        aria-label="Crossfader A/B"
      />
      <span className="xfader-tag">B</span>
    </div>
  );
}
