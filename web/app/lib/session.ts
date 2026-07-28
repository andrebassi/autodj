/* Per-browser session identity.
 *
 * Each browser gets its OWN isolated backend stack (own SQLite file, queue,
 * now-playing, cartwall, grade — nothing shared). The key is a stable browser
 * fingerprint minted by FingerprintJS (OSS) and cached in localStorage. It rides
 * REST calls as the `X-AutoDJ-Session` header and the WebSocket handshake as
 * `?sid=` (browsers can't set custom headers on a WS upgrade).
 *
 * The backend (cmd/serverkit/tenants.go) sanitises this id to [a-z0-9] and maps
 * it to ~/.autodj/sessions/<id>.db. No id → shared "default" tenant.
 */

import FingerprintJS from "@fingerprintjs/fingerprintjs";

const KEY = "autodj:sid";
const HEADER = "X-AutoDJ-Session";

/* RESERVED is the backend's shared-tenant name (cmd/serverkit/tenants.go
 * defaultTenant). A real browser must NEVER persist it as its own id — a stale
 * "default" in localStorage (from an old build or manual debug) would pin this
 * browser to the shared empty tenant forever. Treat it as no-id so initSession
 * re-mints a real fingerprint and overwrites the stale value. */
const RESERVED = "default";

let current = "";
let ready: Promise<string> | null = null;

function cached(): string {
  if (typeof window === "undefined") return "";
  try {
    const v = window.localStorage.getItem(KEY) || "";
    return v === RESERVED ? "" : v;
  } catch {
    return "";
  }
}

/* sessionId returns the current id synchronously (cached value). Empty until
 * initSession resolves on a fresh browser — the fetch wrapper simply skips the
 * header in that window and the caller lands on the shared default tenant. */
export function sessionId(): string {
  if (!current) current = cached();
  return current;
}

/* initSession resolves (and persists) the browser fingerprint exactly once. */
export function initSession(): Promise<string> {
  if (typeof window === "undefined") return Promise.resolve("");
  if (ready) return ready;
  ready = (async () => {
    const existing = cached();
    if (existing) {
      current = existing;
      return existing;
    }
    try {
      const fp = await FingerprintJS.load();
      const { visitorId } = await fp.get();
      current = visitorId;
    } catch {
      // FingerprintJS unavailable → fall back to a random, still-stable id so
      // this browser keeps its own stack across reloads (persisted below).
      current =
        "fp" + Math.random().toString(36).slice(2) + Date.now().toString(36);
    }
    try {
      window.localStorage.setItem(KEY, current);
    } catch {
      /* private mode / storage disabled → stay in-memory for this tab */
    }
    return current;
  })();
  return ready;
}

/* apiBase mirror kept local to avoid an import cycle with api.ts (which imports
 * sessionId from here for wsURL). Same rule: dev :3000 → :8095, else same-origin. */
function base(): string {
  if (typeof window === "undefined") return "";
  const env = process.env.NEXT_PUBLIC_API_BASE;
  if (env) return env;
  if (window.location.port === "3000") return "http://localhost:8095";
  return "";
}

/* isBackend is true only for OUR backend (/api/*). Third-party fetches (Spotify
 * Web Playback SDK, CDNs) must NOT get the header — it would trip CORS preflight. */
function isBackend(url: string): boolean {
  try {
    const b = base();
    if (b) return url.startsWith(b + "/api/");
    if (url.startsWith("/api/")) return true;
    const u = new URL(url, window.location.href);
    return u.origin === window.location.origin && u.pathname.startsWith("/api/");
  } catch {
    return false;
  }
}

let patched = false;

/* patchFetch installs a global window.fetch wrapper that stamps the session
 * header on backend-only requests. Idempotent; safe to call on every render. */
export function patchFetch(): void {
  if (patched || typeof window === "undefined") return;
  patched = true;
  const orig = window.fetch.bind(window);
  window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.href
          : input.url;
    if (isBackend(url)) {
      const id = sessionId();
      if (id) {
        const headers = new Headers(
          init?.headers ?? (input instanceof Request ? input.headers : undefined),
        );
        if (!headers.has(HEADER)) headers.set(HEADER, id);
        init = { ...init, headers };
      }
    }
    return orig(input, init);
  };
}
