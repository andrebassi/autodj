"use client";

import { useEffect, useState } from "react";
import { apiBase } from "@/lib/api";
import type { Program, ProgramRow } from "@/lib/uiContent";
import type { RichRow } from "@/lib/mockProgram";

/* maps a wire ProgramRow ({kind,...}) to the frontend RichRow ({t,...}) */
function toRich(r: ProgramRow): RichRow {
  if (r.kind === "locucao") return { t: "locucao", text: r.text ?? "" };
  return { t: r.kind, label: r.label ?? "", dur: r.dur ?? "" } as RichRow;
}

/* fetches the program preview rows from GET /api/v1/ui/program */
export function useProgram(): { above: RichRow[]; below: RichRow[]; loaded: boolean } {
  const [above, setAbove] = useState<RichRow[]>([]);
  const [below, setBelow] = useState<RichRow[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const res = await fetch(apiBase() + "/api/v1/ui/program");
        const data: Program = await res.json();
        if (!stop && data) {
          setAbove((data.above ?? []).map(toRich));
          setBelow((data.below ?? []).map(toRich));
        }
      } catch {
        /* backend offline → keep empty lists */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  return { above, below, loaded };
}
