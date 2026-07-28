"use client";

import { useEffect, useState } from "react";

/* ticking 1s clock; null until mounted (SSR-safe) */
export function useClock() {
  const [now, setNow] = useState<Date | null>(null);
  useEffect(() => {
    setNow(new Date());
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, []);
  return now;
}
