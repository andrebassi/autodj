"use client";

import { useEffect, useState } from "react";

/* animated cosmetic spectrum bars (decays when inactive) */
export function VuMeter({ active }: { active: boolean }) {
  const bars = 48;
  const [levels, setLevels] = useState<number[]>(() => Array(bars).fill(0.15));
  useEffect(() => {
    const id = setInterval(() => {
      setLevels((prev) =>
        prev.map((v, i) => {
          if (!active) return Math.max(0.05, v * 0.8);
          const target = 0.2 + Math.abs(Math.sin(Date.now() / 300 + i)) * 0.75;
          return v + (target - v) * 0.5;
        })
      );
    }, 90);
    return () => clearInterval(id);
  }, [active]);
  return (
    <div className="vu">
      {levels.map((l, i) => (
        <span
          key={i}
          className="vu-bar"
          style={{ height: `${Math.round(l * 100)}%` }}
        />
      ))}
    </div>
  );
}
