"use client";

import { useEffect, useState } from "react";
import { apiBase } from "@/lib/api";
import { type GradeLoad, EMPTY_HOURLY } from "@/lib/gradeLoad";

/* carga da grade por hora (24 posições). O dado vem da API; enquanto não chega,
   mantém 24 zeros (não inventa carga no front). Mesmo padrão do useProgram. */
export function useGradeLoad(): { hourly: number[]; loaded: boolean } {
  const [hourly, setHourly] = useState<number[]>(EMPTY_HOURLY);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const res = await fetch(apiBase() + "/api/v1/ui/grade-load");
        const data: GradeLoad = await res.json();
        if (!stop && data?.hourly?.length === 24) setHourly(data.hourly);
      } catch {
        /* backend offline → mantém zeros */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  return { hourly, loaded };
}
