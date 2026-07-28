"use client";

import { useEffect, useState, type ReactNode } from "react";
import { initSession, patchFetch, sessionId } from "@/lib/session";

/* SessionBoot resolves the browser fingerprint and installs the fetch wrapper
 * BEFORE any data hook mounts, so every backend call carries X-AutoDJ-Session
 * and lands on this browser's own isolated stack. Children render only once the
 * id is known (instant when already cached in localStorage). */
export default function SessionBoot({ children }: { children: ReactNode }) {
  // patch on first render, before children mount and fire their fetches
  patchFetch();
  const [ready, setReady] = useState(() => sessionId() !== "");

  useEffect(() => {
    let alive = true;
    initSession().then(() => alive && setReady(true));
    return () => {
      alive = false;
    };
  }, []);

  if (!ready) return null;
  return <>{children}</>;
}
