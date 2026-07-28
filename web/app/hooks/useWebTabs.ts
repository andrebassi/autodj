"use client";

import { useEffect, useState } from "react";
import { apiBase } from "@/lib/api";
import type { WebTab } from "@/lib/uiContent";

/* fetches the Internet side-panel tabs from GET /api/v1/ui/webtabs */
export function useWebTabs(): { tabs: WebTab[]; loaded: boolean } {
  const [tabs, setTabs] = useState<WebTab[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let stop = false;
    (async () => {
      try {
        const res = await fetch(apiBase() + "/api/v1/ui/webtabs");
        const data: WebTab[] = await res.json();
        if (!stop && Array.isArray(data)) setTabs(data);
      } catch {
        /* backend offline → keep empty list */
      } finally {
        if (!stop) setLoaded(true);
      }
    })();
    return () => {
      stop = true;
    };
  }, []);

  return { tabs, loaded };
}
