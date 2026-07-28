/* API base + WebSocket URL resolution (dev :3000 → :8095, else same-origin) */

import type { MixConfig } from "@/lib/uiContent";
import { sessionId } from "@/lib/session";
import { humanizeRaw, humanizeResponse, type Provider } from "@/lib/errors";

export function apiBase(): string {
  if (typeof window === "undefined") return "";
  const env = process.env.NEXT_PUBLIC_API_BASE;
  if (env) return env;
  if (window.location.port === "3000") return "http://localhost:8095";
  return "";
}

/* sidQuery carries the browser fingerprint on the WS handshake (no custom
 * headers possible there). Empty id → shared default tenant, so omit it. */
function sidQuery(): string {
  const id = sessionId();
  return id ? "?sid=" + encodeURIComponent(id) : "";
}

export function wsURL(): string {
  const base = apiBase();
  if (base) return base.replace(/^http/, "ws") + "/ws" + sidQuery();
  // Same-origin: derive the ws scheme from the page protocol instead of a naive
  // string replace. In the Wails desktop window the origin is "wails://wails.localhost"
  // (a custom scheme), so replacing /^http/ leaves an invalid "wails://…" URL and
  // `new WebSocket` throws synchronously, crashing the boot. Build ws://host from
  // location.host (host+port) and pick wss only under https.
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/ws` + sidQuery();
}

/* ---- library CRUD ---- */

/* one media category (fixed enum) with pt-BR label + icon */
export type CategoryDTO = { key: string; label: string; ico: string };

/* one cue marker (kind + ms position + optional label) */
export type MarkerDTO = { kind: string; pos_ms: number; label: string };

/* raw track shape from the backend */
export type TrackDTO = {
  id: string;
  title: string;
  artist: string;
  album: string;
  category: string;
  source: string;
  duration_ms: number;
  bpm: number;
  key: string;
  mixable: boolean;
  pcm_available: boolean;
  analyzed: boolean;
  markers: MarkerDTO[];
  spotify_uri?: string;
  play_count?: number;
  /* pastas EFETIVAS: as do operador, ou a do serviço quando é streaming */
  folders?: string[];
  /* já existe arquivo SEM VOCAL preparado por IA? Vem daqui (lido fresco do
     catálogo) e não do Playable da fila, que é um retrato do enfileiramento e
     ficaria dizendo "não" para sempre — a separação termina minutos depois. */
  instrumental_ready?: boolean;
};

/* waveform-editor payload: duration + normalized [0,1] peaks */
export type WaveformDTO = { duration_ms: number; peaks: number[] };

/* fields the operator can edit/set on upload */
export type TrackForm = {
  title?: string;
  artist?: string;
  album?: string;
  category?: string;
  /* pastas aplicadas no envio (valem pro lote inteiro) */
  folders?: string[];
};

export async function fetchCategories(): Promise<CategoryDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/library/categories");
  if (!res.ok) await throwHuman("categories", res);
  return (await res.json()) ?? [];
}

export async function fetchTracks(): Promise<TrackDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/library/tracks");
  if (!res.ok) await throwHuman("tracks", res);
  return (await res.json()) ?? [];
}

/* uploads a file (multipart) + optional metadata → creates a local Track */
export async function uploadTrack(file: File, form: TrackForm): Promise<TrackDTO> {
  const fd = new FormData();
  fd.append("file", file);
  if (form.category) fd.append("category", form.category);
  if (form.title) fd.append("title", form.title);
  if (form.artist) fd.append("artist", form.artist);
  if (form.album) fd.append("album", form.album);
  const res = await fetch(apiBase() + "/api/v1/library/upload", { method: "POST", body: fd });
  if (!res.ok) await throwHuman("upload", res);
  return await res.json();
}

/* uploadTrackProgress envia UM arquivo reportando o progresso real.

   Usa XMLHttpRequest porque `fetch` não expõe progresso de UPLOAD (só de
   download) — e num MP3 de 5 MB numa conexão de estúdio a diferença entre "está
   enviando" e "travou" é justamente essa barra.

   ATENÇÃO: o XHR não passa pelo wrapper que injeta `X-AutoDJ-Session` no fetch, então
   o header vai à mão aqui. Sem ele o upload cai no tenant "default" e o arquivo
   some da estação do operador (mesma armadilha do LOC-1). */
export function uploadTrackProgress(
  file: File,
  form: TrackForm,
  onProgress?: (frac: number) => void,
  signal?: AbortSignal,
): Promise<TrackDTO> {
  return new Promise((resolve, reject) => {
    const fd = new FormData();
    fd.append("file", file);
    if (form.category) fd.append("category", form.category);
    if (form.title) fd.append("title", form.title);
    if (form.artist) fd.append("artist", form.artist);
    if (form.album) fd.append("album", form.album);
    // lista vai como campo repetido: é como multipart representa vários valores
    for (const f of form.folders ?? []) fd.append("folder", f);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", apiBase() + "/api/v1/library/upload", true);
    const sid = sessionId();
    if (sid) xhr.setRequestHeader("X-AutoDJ-Session", sid);

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(e.loaded / e.total);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress?.(1);
        try {
          resolve(JSON.parse(xhr.responseText) as TrackDTO);
        } catch {
          reject(new Error("resposta inválida do servidor"));
        }
        return;
      }
      // passa pelo MESMO humanizador do resto da API: o motivo aparece por arquivo
      // na lista do envio, e "exit status 1" ali não ajuda ninguém.
      let raw = "";
      try {
        const b = JSON.parse(xhr.responseText) as { error?: string };
        raw = b?.error ?? "";
      } catch {
        /* corpo não-JSON: humaniza só pelo status */
      }
      reject(new Error(humanizeRaw(raw, xhr.status)));
    };
    xhr.onerror = () => reject(new Error("falha de rede no envio"));
    xhr.onabort = () => reject(new DOMException("cancelado", "AbortError"));
    signal?.addEventListener("abort", () => xhr.abort(), { once: true });
    xhr.send(fd);
  });
}


/* ---- Pastas do operador (+ automáticas de fonte) --------------------------
   `auto` marca as que NÃO são editáveis: Spotify, YouTube e "Sem pasta". */
export type FolderDTO = { name: string; count: number; auto: boolean };

export async function listFolders(): Promise<FolderDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/library/folders");
  if (!res.ok) await throwHuman("listFolders", res);
  return ((await res.json()) ?? []) as FolderDTO[];
}

export async function createFolder(name: string): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/library/folders", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) await throwHuman("createFolder", res);
}

export async function deleteFolder(name: string): Promise<void> {
  const res = await fetch(
    apiBase() + "/api/v1/library/folders/" + encodeURIComponent(name),
    { method: "DELETE" },
  );
  if (!res.ok) await throwHuman("deleteFolder", res);
}

/* troca as pastas de UMA faixa. Streaming responde 409 (pasta fixa do serviço). */
export async function setTrackFolders(id: string, folders: string[]): Promise<TrackDTO> {
  const res = await fetch(
    apiBase() + "/api/v1/library/tracks/" + encodeURIComponent(id) + "/folders",
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ folders }),
    },
  );
  if (!res.ok) await throwHuman("setTrackFolders", res);
  return (await res.json()) as TrackDTO;
}

export async function updateTrack(id: string, form: TrackForm): Promise<TrackDTO> {
  const res = await fetch(apiBase() + "/api/v1/library/tracks/" + encodeURIComponent(id), {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(form),
  });
  if (!res.ok) await throwHuman("update", res);
  return await res.json();
}

export async function deleteTrack(id: string): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/library/tracks/" + encodeURIComponent(id), {
    method: "DELETE",
  });
  if (!res.ok && res.status !== 204) await throwHuman("delete", res);
}

/* ---- markers + waveform (PCM only) ---- */

/* replaces the cue markers of a PCM track (422 if the track is DRM/Spotify) */
export async function setMarkers(id: string, markers: MarkerDTO[]): Promise<TrackDTO> {
  const res = await fetch(
    apiBase() + "/api/v1/library/tracks/" + encodeURIComponent(id) + "/markers",
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ markers }),
    },
  );
  if (!res.ok) await throwHuman("markers", res);
  return await res.json();
}

/* fetches the waveform peaks of a PCM track (bars = requested resolution) */
export async function fetchWaveform(id: string, bars = 160): Promise<WaveformDTO> {
  const res = await fetch(
    apiBase() +
      "/api/v1/library/tracks/" +
      encodeURIComponent(id) +
      "/waveform?bars=" +
      bars,
  );
  if (!res.ok) await throwHuman("waveform", res);
  return await res.json();
}

/* raw-audio stream URL for a mixable PCM track — the DJ deck fetches this and
   decodes it client-side (Story 5.1). Backend serves 403 for non-mixable
   (Spotify/DRM) sources, so only pass mixable && pcm_available tracks. */
export function trackAudioURL(id: string, instrumental = false): string {
  // `<audio src>` NÃO carrega o header X-AutoDJ-Session (só o fetch é embrulhado),
  // exatamente como o `<img>` da capa — então a sessão vai por ?sid=. Sem isto o
  // request cai no tenant "default", que não tem a faixa desta estação, e responde
  // 404: o elemento falha, o play() rejeita e o operador vê a faixa parada em 00:00
  // SEM mensagem nenhuma (o .catch do playout engole). O artwork já fazia certo; o
  // áudio ficou pra trás. Pegadinha ao depurar: o cache HTTP do browser não
  // distingue header, então um fetch anterior "conserta" o <audio> por um tempo e
  // esconde o bug.
  /* `instrumental` pede a versão SEM VOCAL, quando a separação por IA já rodou
     nesta faixa. É um parâmetro na MESMA rota de propósito: o `<audio>` do deck
     já negocia Range/seek com ela, o Service Worker já a cacheia e o buffer
     anti-queda já a conhece. Uma rota nova teria de reconquistar tudo isso.
     Sem o preparo, o servidor devolve o original — nunca 404 no meio da música. */
  const stem = instrumental ? "&stem=instrumental" : "";
  const q = sidQuery();
  return (
    apiBase() +
    "/api/v1/library/tracks/" +
    encodeURIComponent(id) +
    "/audio" +
    (q ? q + stem : stem.replace("&", "?"))
  );
}

/** Estado da separação por IA de uma faixa. */
export type StemStatus = "idle" | "queued" | "running" | "ready" | "failed";

/* prepareInstrumental pede a separação e devolve o estado inicial.
   `null` = a ferramenta não existe nesta máquina (rota não registrada, 404) —
   a UI esconde a função em vez de mostrar erro. */
export async function prepareInstrumental(id: string): Promise<StemStatus | null> {
  const r = await fetch(
    apiBase() + "/api/v1/library/tracks/" + encodeURIComponent(id) + "/instrumental",
    { method: "POST" },
  );
  if (r.status === 404) return null;
  if (!r.ok) await throwHuman("prepareInstrumental", r);
  return ((await r.json()) as { status: StemStatus }).status;
}

/* instrumentalsWorking lista o que está sendo preparado AGORA, por id — UMA
   requisição para a lista inteira, em vez de uma por linha. Mapa vazio = nada em
   andamento. `null` quando a rota não existe (ferramenta não instalada): a UI
   simplesmente não desenha barra nenhuma. */
export async function instrumentalsWorking(): Promise<Record<
  string,
  StemStatus
> | null> {
  const r = await fetch(apiBase() + "/api/v1/library/instrumentals");
  if (r.status === 404) return null;
  if (!r.ok) await throwHuman("instrumentalsWorking", r);
  return (await r.json()) as Record<string, StemStatus>;
}

/* instrumentalStatus acompanha o andamento. Leva MINUTOS (a separação roda a
   ~1,4× o tempo real), então a UI faz poll — não é uma chamada que espera. */
export async function instrumentalStatus(
  id: string,
): Promise<{ status: StemStatus; error: string } | null> {
  const r = await fetch(
    apiBase() + "/api/v1/library/tracks/" + encodeURIComponent(id) + "/instrumental",
  );
  if (r.status === 404) return null;
  if (!r.ok) await throwHuman("instrumentalStatus", r);
  return (await r.json()) as { status: StemStatus; error: string };
}

// trackArtworkURL points at the cover endpoint for ANY provider (hexagonal): the
// server resolves it per-source (Spotify album image via redirect, local embedded
// picture, …). Empty id → "" (nothing to render). The <img> onError falls back to
// a placeholder tile, so a 404 (track with no cover) degrades gracefully.
export function trackArtworkURL(id: string): string {
  if (!id) return "";
  // <img src> can't carry the X-AutoDJ-Session header (only fetch is wrapped), so the
  // session rides as ?sid= — otherwise the cover lands on the default tenant, which
  // doesn't hold this browser's imported track, and 404s to the placeholder tile.
  return (
    apiBase() +
    "/api/v1/library/tracks/" +
    encodeURIComponent(id) +
    "/artwork" +
    sidQuery()
  );
}

/* ---- fine-mixing settings (station-wide, Story 3.3) ---- */

/* current persisted fine-mixing settings (backend returns defaults if unset) */
export async function fetchMix(): Promise<MixConfig> {
  const res = await fetch(apiBase() + "/api/v1/mix");
  if (!res.ok) await throwHuman("mix", res);
  return await res.json();
}

/* validates + persists the fine-mixing settings; returns the stored value.
   Throws on 422 (out-of-range value) or other non-2xx. */
export async function saveMix(mix: MixConfig): Promise<MixConfig> {
  const res = await fetch(apiBase() + "/api/v1/mix", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(mix),
  });
  if (!res.ok) await throwHuman("saveMix", res);
  return await res.json();
}

/* ---- clocks / hour templates (station programming, Story 4.1) ---- */

/* one slot of a clock: kind (music/jingle/commercial/event) + media category +
   how many items of that shape the slot contributes to the hour */
export type SlotDTO = { kind: string; category: string; count: number };

/* one clock / hour template: id + name + ordered slots */
export type ClockDTO = { id: string; name: string; slots: SlotDTO[] };

/* all station clocks (backend seeds defaults on first run) */
export async function fetchClocks(): Promise<ClockDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/clocks");
  if (!res.ok) await throwHuman("clocks", res);
  return (await res.json()) ?? [];
}

/* validates + upserts a clock (PUT /clocks/{id}); the path id is authoritative.
   Returns the stored value. Throws on 422 (incoherent clock) or other non-2xx. */
export async function saveClock(clock: ClockDTO): Promise<ClockDTO> {
  const res = await fetch(apiBase() + "/api/v1/clocks/" + encodeURIComponent(clock.id), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(clock),
  });
  if (!res.ok) await throwHuman("saveClock", res);
  return await res.json();
}

/* removes a clock by id (404 if unknown) */
export async function deleteClock(id: string): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/clocks/" + encodeURIComponent(id), {
    method: "DELETE",
  });
  if (!res.ok && res.status !== 204) await throwHuman("deleteClock", res);
}

/* generates a preview playlist from a clock's rotation rules (POST
   /clocks/{id}/generate). No track is played — read-only preview. Returns the
   ordered tracks. Throws on 404 (unknown clock) or 422 (empty catalog). */
export async function generatePlaylist(clockId: string): Promise<TrackDTO[]> {
  const res = await fetch(
    apiBase() + "/api/v1/clocks/" + encodeURIComponent(clockId) + "/generate",
    { method: "POST" },
  );
  if (!res.ok) await throwHuman("generatePlaylist", res);
  return (await res.json()) ?? [];
}

/* one grid cell: weekday (0=Sun..6=Sat) + hour (0..23) → clock id */
export type CellDTO = { weekday: number; hour: number; clockId: string };

/* the whole weekly programming grid (sparse: only programmed cells) */
export type GridDTO = { cells: CellDTO[] };

/* the weekly grid (backend seeds a default 7×24 on first run) */
export async function fetchGrid(): Promise<GridDTO> {
  const res = await fetch(apiBase() + "/api/v1/grid");
  if (!res.ok) await throwHuman("grid", res);
  return (await res.json()) ?? { cells: [] };
}

/* validates + persists the whole grid (PUT /grid). Returns the stored value.
   Throws on 422 (bad weekday/hour, duplicate cell, unknown clock) or other non-2xx. */
export async function saveGrid(grid: GridDTO): Promise<GridDTO> {
  const res = await fetch(apiBase() + "/api/v1/grid", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(grid),
  });
  if (!res.ok) await throwHuman("saveGrid", res);
  return await res.json();
}

/* one scheduled event: recurrence (days + hour:minute) + kind + priority + the
   media it injects. mediaRef resolves by exact track id, else a media category.
   Empty mediaRef = generated later (e.g. TTS hora certa). */
export type ScheduledEventDTO = {
  id: string;
  kind: string;
  days: number[]; // 0=Sun..6=Sat
  hour: number; // 0..23
  minute: number; // 0..59
  priority: number; // 0..100 (higher preempts)
  mediaRef: string;
};

/* the scheduled event kinds the backend recognizes (EventKind enum) */
export const EVENT_KINDS = [
  "voz_do_brasil",
  "hora_certa",
  "temperatura",
  "comercial",
  "programete",
] as const;

/* all station events (backend seeds defaults on first run) */
export async function fetchEvents(): Promise<ScheduledEventDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/events");
  if (!res.ok) await throwHuman("events", res);
  return (await res.json()) ?? [];
}

/* validates + upserts an event (PUT /events/{id}); the path id is authoritative.
   Returns the stored value. Throws on 422 (incoherent event) or other non-2xx. */
export async function saveEvent(ev: ScheduledEventDTO): Promise<ScheduledEventDTO> {
  const res = await fetch(apiBase() + "/api/v1/events/" + encodeURIComponent(ev.id), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(ev),
  });
  if (!res.ok) await throwHuman("saveEvent", res);
  return await res.json();
}

/* removes an event by id (404 if unknown) */
export async function deleteEvent(id: string): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/events/" + encodeURIComponent(id), {
    method: "DELETE",
  });
  if (!res.ok && res.status !== 204) await throwHuman("deleteEvent", res);
}

/* resolves the events due at instant `at` into the ordered media to inject
   (preemption preview, priority-first). No track is played. `at` is RFC3339. */
export async function eventsDue(at: string): Promise<TrackDTO[]> {
  const res = await fetch(
    apiBase() + "/api/v1/events/due?at=" + encodeURIComponent(at),
  );
  if (!res.ok) await throwHuman("eventsDue", res);
  return (await res.json()) ?? [];
}

/* ---- Cartwall (Story 5.4): instant-fire button board, per user ---- */

/* one cartwall pad. trackId points at a library track (must be mixable/PCM —
   the backend enforces the two-engine guard); hue is a 0..360 colour hint;
   hotkey is a single key (empty = none); page groups buttons into tabs (≥1). */
export type CartButtonDTO = {
  id: string;
  label: string;
  trackId: string;
  hue: number;
  icon: string;
  hotkey: string;
  page: number;
};

/* the whole board = flat list of buttons partitioned by page */
export type CartBoardDTO = { buttons: CartButtonDTO[] };

/* the operator's cartwall board (backend serves Default() when the store is
   empty, so this never comes back without buttons on first run) */
export async function fetchCartwall(): Promise<CartBoardDTO> {
  const res = await fetch(apiBase() + "/api/v1/cartwall");
  if (!res.ok) await throwHuman("cartwall", res);
  return (await res.json()) ?? { buttons: [] };
}

/* validates + persists the whole board (PUT /cartwall). Returns the stored
   value. Throws on 409 (hotkey clash) or 422 (non-mixable/empty) or other. */
export async function saveCartwall(board: CartBoardDTO): Promise<CartBoardDTO> {
  const res = await fetch(apiBase() + "/api/v1/cartwall", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(board),
  });
  if (!res.ok) await throwHuman("saveCartwall", res);
  return await res.json();
}

/* upserts a single button (POST /cartwall/buttons) and returns the whole
   updated board. Throws HTTPError with .status so the UI can tell 409 (hotkey
   taken) from 422 (non-mixable/empty label). */
export async function assignCartButton(btn: CartButtonDTO): Promise<CartBoardDTO> {
  const res = await fetch(apiBase() + "/api/v1/cartwall/buttons", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(btn),
  });
  if (!res.ok) await throwHuman("assignCartButton", res);
  return await res.json();
}

/* removes a button by id (DELETE /cartwall/buttons/{id}); 404 if unknown */
export async function deleteCartButton(id: string): Promise<void> {
  const res = await fetch(
    apiBase() + "/api/v1/cartwall/buttons/" + encodeURIComponent(id),
    { method: "DELETE" },
  );
  if (!res.ok && res.status !== 204) await throwHuman("deleteCartButton", res);
}

/* an error carrying the HTTP status so callers can branch on 409 vs 422.
   `message` is user-facing: pass a humanized PT-BR string when there is one
   (see lib/errors.ts), else it falls back to the terse "op: status". Callers
   that branch (useSocial) read `.status`, never the text — so a friendly
   message never breaks status logic. `raw` keeps the backend {error} body for
   debugging/telemetry when a humanized message replaced it. */
export class HTTPError extends Error {
  status: number;
  raw?: string;
  constructor(op: string, status: number, friendly?: string, raw?: string) {
    super(friendly ?? op + ": " + status);
    this.name = "HTTPError";
    this.status = status;
    this.raw = raw;
  }
}

/* throwHuman: ponto ÚNICO de erro de rede. Lê o corpo {error} do backend, produz
   uma mensagem PT-BR humanizada (lib/errors.ts) e lança um HTTPError com o status
   preservado (useSocial branqueia por .status, nunca pelo texto) + a `.message`
   amigável que os painéis exibem. Substitui os antigos `throw new Error("op:500")`
   crus. `provider` afina o texto (spotify/youtube). Retorna Promise<never>. */
async function throwHuman(
  op: string,
  res: Response,
  provider: Provider = "",
): Promise<never> {
  const { status, message, raw } = await humanizeResponse(res, provider);
  throw new HTTPError(op, status, message, raw);
}

/* ---- Autopilot (Story 4.5): unattended supervision status + toggle ---- */

export type AutopilotStatus = {
  enabled: boolean;
  queueLen: number;
  lastTick: string; // RFC3339; zero time when never ticked
};

/* reads GET /api/v1/autopilot — live supervision state */
export async function fetchAutopilot(): Promise<AutopilotStatus> {
  const res = await fetch(apiBase() + "/api/v1/autopilot");
  if (!res.ok) await throwHuman("fetchAutopilot", res);
  return await res.json();
}

/* POSTs the enabled flag; enabling ticks once immediately server-side. Returns
   the post-tick status. */
export async function setAutopilot(enabled: boolean): Promise<AutopilotStatus> {
  const res = await fetch(apiBase() + "/api/v1/autopilot", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
  if (!res.ok) await throwHuman("setAutopilot", res);
  return await res.json();
}

/* ---- Spotify OAuth (Story 6.1): connection status + PKCE connect/disconnect ----
   Routes exist only when the backend has AUTODJ_SPOTIFY_CLIENT_ID set; a 404
   means the integration is unconfigured (handled by the hook as `configured=false`). */

export type SpotifyStatus = {
  connected: boolean;
  premium: boolean;
};

/* reads GET /api/v1/spotify/status. A 404 signals the integration is not
   configured on the server — surfaced via the SpotifyNotConfigured marker. */
export const SpotifyNotConfigured = Symbol("spotify-not-configured");

export async function fetchSpotifyStatus(): Promise<
  SpotifyStatus | typeof SpotifyNotConfigured
> {
  const res = await fetch(apiBase() + "/api/v1/spotify/status");
  if (res.status === 404) return SpotifyNotConfigured;
  if (!res.ok) await throwHuman("fetchSpotifyStatus", res);
  return await res.json();
}

/* POSTs /connect → returns the accounts.spotify.com authorize URL to navigate to.
 * The OAuth callback comes back as a top-level navigation (no session header), so we
 * smuggle this browser's fingerprint into the `state` param as "<sid>~<state>". The
 * backend dispatcher peels the sid off to route the callback to the right tenant. */
export async function spotifyConnect(): Promise<string> {
  const res = await fetch(apiBase() + "/api/v1/spotify/connect", {
    method: "POST",
  });
  if (!res.ok) await throwHuman("spotifyConnect", res);
  const body = await res.json();
  const authURL = body.authURL as string;
  const sid = sessionId();
  if (!sid) return authURL;
  const u = new URL(authURL);
  const st = u.searchParams.get("state");
  if (st) {
    u.searchParams.set("state", sid + "~" + st);
    return u.toString();
  }
  return authURL;
}

/* Diagnóstico do Spotify: cadeia de verificação em ordem causal (conta → token →
   perfil → premium → aparelho). O primeiro degrau que falha é a causa. */
export type SpotifyStep = {
  key: string;
  label: string;
  ok: boolean;
  detail: string;
  hint?: string;
};
export type SpotifyDiagnosis = {
  steps: SpotifyStep[];
  ok: boolean;
  verdict?: string;
};

/* GETs /diagnose → passo a passo legível. Só leitura, nunca inicia reprodução. */
export async function spotifyDiagnose(): Promise<SpotifyDiagnosis> {
  const res = await fetch(apiBase() + "/api/v1/spotify/diagnose");
  if (!res.ok) await throwHuman("spotifyDiagnose", res);
  return (await res.json()) as SpotifyDiagnosis;
}

/* POSTs /revalidate → força renovação do token e devolve o estado REAL da conta.
   Chamado quando o playout trava: device conectado, faixa carregada, comandos
   aceitos e áudio nenhum. Devolve null quando a renovação falha (token recusado
   pelo Spotify) — aí a conta precisa ser reconectada. */
export async function spotifyRevalidate(): Promise<SpotifyStatus | null> {
  const res = await fetch(apiBase() + "/api/v1/spotify/revalidate", {
    method: "POST",
  });
  if (!res.ok) return null;
  return (await res.json()) as SpotifyStatus;
}

/* POSTs /disconnect → unlinks the account (idempotent). */
export async function spotifyDisconnect(): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/spotify/disconnect", {
    method: "POST",
  });
  if (!res.ok) await throwHuman("spotifyDisconnect", res);
}

/* ---- Spotify catalog (Story 6.2): search + import as non-mixable tracks ----
   Metadata only — imports never yield PCM (mixable stays false). */

export type SpotifyKind = "track" | "playlist";

/* a metadata-only search result. For a playlist, artist is the owner and
   durationMs/album are zero. */
export type CatalogItem = {
  uri: string;
  title: string;
  artist: string;
  album: string;
  durationMs: number;
  kind: SpotifyKind;
  artworkUrl?: string; // album cover (Spotify), "" quando ausente → placeholder
};

/* GET /spotify/search?q&type → catalog items ([] when nothing matches). */
export async function spotifySearch(
  query: string,
  kind: SpotifyKind,
): Promise<CatalogItem[]> {
  const q = new URLSearchParams({ q: query, type: kind });
  const res = await fetch(apiBase() + "/api/v1/spotify/search?" + q.toString());
  if (!res.ok) await throwHuman("spotifySearch", res, "spotify");
  return await res.json();
}

/* POST /spotify/import → imports track URIs (or a whole playlist) as
   non-mixable Spotify tracks, returning the created tracks. Erros (playlist Dev
   404/403, quota, etc.) humanizados por throwHuman(..., "spotify"). */
export async function spotifyImport(
  req: { uris?: string[]; playlistID?: string },
): Promise<TrackDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/spotify/import", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) await throwHuman("spotifyImport", res, "spotify");
  return await res.json();
}

/* imports a Spotify URI (track or playlist) into the library and returns the
   created track(s). Picks playlistID vs uris by inspecting the URI form
   (spotify:playlist:… → whole playlist; anything else → single track). Used to
   auto-import a search result on enqueue/drag before it joins the playout queue
   as a non-mixable Spotify track (it plays out-of-band via the SDK). */
export async function importSpotifyURI(uri: string): Promise<TrackDTO[]> {
  const req = uri.includes(":playlist:") ? { playlistID: uri } : { uris: [uri] };
  return await spotifyImport(req);
}

/* fetches the current Spotify access token for the Web Playback SDK (Story 6.3).
   The browser SDK's getOAuthToken callback calls this; the backend refreshes it
   transparently. 409 = no linked account (connect first). This token authorizes
   only Spotify's own SDK playback — no PCM ever reaches the local bus. */
export async function fetchSpotifyToken(): Promise<string> {
  const res = await fetch(apiBase() + "/api/v1/spotify/token");
  if (!res.ok) await throwHuman("fetchSpotifyToken", res);
  const body = (await res.json()) as { accessToken: string };
  return body.accessToken;
}

/* pauses the account's ACTIVE Spotify device via the Connect Web API, whatever it
   is — the AutoDJ Playout SDK device OR another phone/desktop the user is playing
   on. The Web Playback SDK's own player.pause() only silences its own device, so
   when playback took over to another device the master STOP couldn't silence it
   ("dei stop e a música continuou no outro aparelho"). This is authoritative: the
   OAuth token carries user-modify-playback-state. Best-effort — 404 (no active
   device) / 403 (no Premium) are non-fatal; STOP still stops everything local. */
export async function spotifyConnectPause(): Promise<void> {
  let token: string;
  try {
    token = await fetchSpotifyToken();
  } catch {
    return; // no linked Spotify account → nothing to pause remotely
  }
  try {
    await fetch("https://api.spotify.com/v1/me/player/pause", {
      method: "PUT",
      headers: { Authorization: "Bearer " + token },
    });
  } catch {
    /* rede/CORS/404/403 → ignora; STOP local já cuidou do resto */
  }
}

/* ---- YouTube OAuth (Story 6.5): connection status + Google connect/disconnect ----
   Espelho fiel do Spotify. Rotas existem só quando o backend tem
   AUTODJ_YOUTUBE_CLIENT_ID + _CLIENT_SECRET + _API_KEY setados; um 404 significa
   integração não configurada (o hook trata como `configured=false`). */

export type YouTubeStatus = {
  connected: boolean;
  premium: boolean; // best-effort no YouTube (não mapeia) — só p/ paridade do chip
  freeSearch?: boolean; // servidor tem o backend grátis (InnerTube) ligado
  oauth?: boolean; // servidor tem client id/secret — sem isso "Minha conta" é impossível
};

/* reads GET /api/v1/youtube/status. A 404 signals the integration is not
   configured on the server — surfaced via the YouTubeNotConfigured marker. */
export const YouTubeNotConfigured = Symbol("youtube-not-configured");

export async function fetchYouTubeStatus(): Promise<
  YouTubeStatus | typeof YouTubeNotConfigured
> {
  const res = await fetch(apiBase() + "/api/v1/youtube/status");
  if (res.status === 404) return YouTubeNotConfigured;
  if (!res.ok) await throwHuman("fetchYouTubeStatus", res);
  return await res.json();
}

/* POSTs /connect → returns the accounts.google.com authorize URL to navigate to.
 * O callback OAuth volta como navegação top-level (sem header de sessão), então
 * contrabandeamos o fingerprint deste browser no `state` como "<sid>~<state>". O
 * dispatcher do backend descasca o sid p/ rotear o callback ao tenant certo. */
export async function youtubeConnect(): Promise<string> {
  const res = await fetch(apiBase() + "/api/v1/youtube/connect", {
    method: "POST",
  });
  if (!res.ok) await throwHuman("youtubeConnect", res);
  const body = await res.json();
  const authURL = body.authURL as string;
  const sid = sessionId();
  if (!sid) return authURL;
  const u = new URL(authURL);
  const st = u.searchParams.get("state");
  if (st) {
    u.searchParams.set("state", sid + "~" + st);
    return u.toString();
  }
  return authURL;
}

/* POSTs /disconnect → unlinks the account (idempotent). */
export async function youtubeDisconnect(): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/youtube/disconnect", {
    method: "POST",
  });
  if (!res.ok) await throwHuman("youtubeDisconnect", res);
}

/* ---- YouTube catalog (Story 6.6): search + import as non-mixable tracks ----
   Metadata only — imports never yield PCM (mixable stays false). */

export type YouTubeKind = "video" | "playlist";

/* a metadata-only search result. For a playlist, artist is the channel and
   durationMs/album are zero. */
export type YouTubeItem = {
  uri: string;
  title: string;
  artist: string; // channel title
  album: string; // always "" for YouTube
  durationMs: number;
  kind: YouTubeKind;
  artworkUrl?: string; // thumbnail (YouTube), "" quando ausente → placeholder
};

/* qual motor atende a busca. "free" = InnerTube (o mesmo protocolo do site do
   YouTube): sem login, sem API key, sem gastar cota. "api" = YouTube Data API v3:
   exige conta conectada (o login é escudo da cota) e cada busca custa ~100 das
   ~10k unidades/dia. Os dois convivem — o operador chaveia no painel. */
export type YouTubeBackend = "free" | "api";

/* GET /youtube/search?q&type&backend → catalog items ([] when nothing matches). */
export async function youtubeSearch(
  query: string,
  kind: YouTubeKind,
  backend: YouTubeBackend = "free",
): Promise<YouTubeItem[]> {
  const q = new URLSearchParams({ q: query, type: kind, backend });
  const res = await fetch(apiBase() + "/api/v1/youtube/search?" + q.toString());
  if (!res.ok) await throwHuman("youtubeSearch", res, "youtube");
  return await res.json();
}

/* POST /youtube/import → imports video URIs (or a whole playlist) as
   non-mixable YouTube tracks, returning the created tracks. Erros (quota 429,
   conta desconectada, etc.) humanizados por throwHuman(..., "youtube"). */
export async function youtubeImport(
  req: { uris?: string[]; playlistID?: string },
): Promise<TrackDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/youtube/import", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) await throwHuman("youtubeImport", res, "youtube");
  return await res.json();
}

/* POST /youtube/import-links → importa vídeos a partir de texto colado (links
   separados por ";", vírgula ou quebra de linha). NÃO exige conta conectada: o
   operador já tem os links, e resolver ids custa 1 unidade da Data API por lote
   (a busca, que custa ~100, continua atrás do login). Texto sem link
   reconhecível → 400 humanizado. Vídeos não-incorporáveis são descartados pelo
   catálogo, então a resposta pode vir menor que o colado. */
export async function youtubeImportLinks(text: string): Promise<TrackDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/youtube/import-links", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
  });
  if (!res.ok) await throwHuman("youtubeImportLinks", res, "youtube");
  return await res.json();
}

/* imports a YouTube URI (video or playlist) into the library and returns the
   created track(s). Picks playlistID vs uris by inspecting the URI form
   (…list=… → whole playlist; anything else → single video). Used to auto-import
   a search result on enqueue/drag before it joins the playout queue as a
   non-mixable YouTube track (it plays out-of-band via the IFrame player). */
export async function importYouTubeURI(uri: string): Promise<TrackDTO[]> {
  const req = uri.includes("list=") ? { playlistID: uri } : { uris: [uri] };
  return await youtubeImport(req);
}

/* fetches the current YouTube access token (Story 6.5). Kept for parity with
   Spotify; the IFrame player does not need it, so this is best-effort. 409 = no
   linked account (connect first). */
export async function fetchYouTubeToken(): Promise<string> {
  const res = await fetch(apiBase() + "/api/v1/youtube/token");
  if (!res.ok) await throwHuman("fetchYouTubeToken", res);
  const body = (await res.json()) as { accessToken: string };
  return body.accessToken;
}

/* ---- AI DJ spoken segues (Story 6.4): config + voiceover ----
   The synthesized voice is source=tts — PCM, mixable — so it rides the local bus
   and bridges a DRM Spotify track (Spotify SDK only fades its own volume). Routes
   exist only when the backend wired an AIDJService; a 404 means unconfigured. */

/* the AI DJ voices offered by the panel (fictional ids, anti-copyright). */
export type AIDJVoice = "aurora-ptbr" | "nimbus-ptbr" | "faber-ptbr";

export const AIDJ_VOICES: { value: AIDJVoice; label: string }[] = [
  { value: "aurora-ptbr", label: "Aurora (pt-BR)" },
  { value: "nimbus-ptbr", label: "Nimbus (pt-BR)" },
  { value: "faber-ptbr", label: "Faber (pt-BR)" },
];

export type AIDJConfig = {
  enabled: boolean;
  voice: AIDJVoice;
};

/* a 404 signals the AI DJ integration is not wired on the server. */
export const AIDJNotConfigured = Symbol("aidj-not-configured");

/* reads GET /api/v1/aidj/config. 404 → AIDJNotConfigured. */
export async function fetchAIDJConfig(): Promise<
  AIDJConfig | typeof AIDJNotConfigured
> {
  const res = await fetch(apiBase() + "/api/v1/aidj/config");
  if (res.status === 404) return AIDJNotConfigured;
  if (!res.ok) await throwHuman("fetchAIDJConfig", res);
  return await res.json();
}

/* PUT /api/v1/aidj/config → stores enabled + voice, returns the stored config.
   422 = unknown voice. */
export async function setAIDJConfig(cfg: AIDJConfig): Promise<AIDJConfig> {
  const res = await fetch(apiBase() + "/api/v1/aidj/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(cfg),
  });
  if (!res.ok) await throwHuman("setAIDJConfig", res);
  return await res.json();
}

/* POST /api/v1/aidj/voiceover → renders + synthesizes the spoken segue for the
   next track, returning a source=tts mixable Track to enqueue ahead of it.
   409 = spoken segues off; 503 = no TTS gateway wired; 404 = unknown track. */
export async function aidjVoiceover(trackId: string): Promise<TrackDTO> {
  const res = await fetch(apiBase() + "/api/v1/aidj/voiceover", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ trackId }),
  });
  if (!res.ok) await throwHuman("aidjVoiceover", res);
  return await res.json();
}

/* ---- RDS (Story 7.1) ---------------------------------------------------
   Radio Data System sidecar: fixed station identity (PI/PS/PTY) + dynamic
   RadioText derived from now-playing metadata. RDS carries text only, no audio,
   so it never touches the two-engine invariant. The /rds routes exist only when
   the backend wired an RDSService; a 404 means unconfigured. */

export type RDSConfig = {
  enabled: boolean;
  pi: number;
  ps: string;
  pty: number;
};

export type RDSState = {
  pi: number;
  ps: string;
  pty: number;
  rt: string;
  tp: boolean;
  ta: boolean;
  ms: boolean;
};

/* a 404 signals the RDS sidecar is not wired on the server. */
export const RDSNotConfigured = Symbol("rds-not-configured");

/* reads GET /api/v1/rds/config. 404 → RDSNotConfigured. */
export async function fetchRDSConfig(): Promise<
  RDSConfig | typeof RDSNotConfigured
> {
  const res = await fetch(apiBase() + "/api/v1/rds/config");
  if (res.status === 404) return RDSNotConfigured;
  if (!res.ok) await throwHuman("fetchRDSConfig", res);
  return await res.json();
}

/* PUT /api/v1/rds/config → stores the identity, returns the stored config.
   422 = bad identity (PI/PS/PTY out of the RDS limits). */
export async function setRDSConfig(cfg: RDSConfig): Promise<RDSConfig> {
  const res = await fetch(apiBase() + "/api/v1/rds/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(cfg),
  });
  if (!res.ok) await throwHuman("setRDSConfig", res);
  return await res.json();
}

/* GET /api/v1/rds/state → the last state pushed to the encoder. */
export async function fetchRDSState(): Promise<RDSState> {
  const res = await fetch(apiBase() + "/api/v1/rds/state");
  if (!res.ok) await throwHuman("fetchRDSState", res);
  return await res.json();
}

/* POST /api/v1/rds/refresh → rebuilds RDS state from now-playing metadata and
   pushes it, returning the pushed state. 409 = RDS off; 422 = bad identity;
   503 = no encoder wired. */
export async function refreshRDS(
  title: string,
  artist: string,
): Promise<RDSState> {
  const res = await fetch(apiBase() + "/api/v1/rds/refresh", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, artist }),
  });
  if (!res.ok) await throwHuman("refreshRDS", res);
  return await res.json();
}

/* ---- Line-in (Story 7.2) ----------------------------------------------
   Live-capture inputs (mic/aux/soundcard): the legitimate *live* PCM source,
   always mixable, never DRM — the opposite of Spotify. Arm routes an input onto
   the local bus ducking the music; gain/duck tune the mix. The /linein routes
   exist only when the backend wired a LineInService; a 404 list means unwired. */

export type LineInInput = {
  id: string;
  label: string;
  armed: boolean;
  gain: number;
  duck: number;
};

/* a 404 signals no line-in controller is wired on the server. */
export const LineInNotConfigured = Symbol("linein-not-configured");

/* GET /api/v1/linein → the configured inputs. 404 → LineInNotConfigured. */
export async function fetchLineIn(): Promise<
  LineInInput[] | typeof LineInNotConfigured
> {
  const res = await fetch(apiBase() + "/api/v1/linein");
  if (res.status === 404) return LineInNotConfigured;
  if (!res.ok) await throwHuman("fetchLineIn", res);
  return await res.json();
}

/* POST /api/v1/linein/{id}/arm → routes the input onto the bus. 404 = unknown. */
export async function armLineIn(id: string): Promise<LineInInput> {
  const res = await fetch(
    apiBase() + "/api/v1/linein/" + encodeURIComponent(id) + "/arm",
    { method: "POST" },
  );
  if (!res.ok) await throwHuman("armLineIn", res);
  return await res.json();
}

/* POST /api/v1/linein/{id}/disarm → drops the input from the bus (idempotent). */
export async function disarmLineIn(id: string): Promise<LineInInput> {
  const res = await fetch(
    apiBase() + "/api/v1/linein/" + encodeURIComponent(id) + "/disarm",
    { method: "POST" },
  );
  if (!res.ok) await throwHuman("disarmLineIn", res);
  return await res.json();
}

/* PUT /api/v1/linein/{id}/levels → sets gain and/or duck ([0,1]). Only the
   fields supplied are applied. 422 = out of range; 404 = unknown id. */
export async function setLineInLevels(
  id: string,
  levels: { gain?: number; duck?: number },
): Promise<LineInInput> {
  const res = await fetch(
    apiBase() + "/api/v1/linein/" + encodeURIComponent(id) + "/levels",
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(levels),
    },
  );
  if (!res.ok) await throwHuman("setLineInLevels", res);
  return await res.json();
}

/* ---- Auth / RBAC (Story 7.3) ------------------------------------------
   Operator accounts gated by the fixed role→permission matrix (admin /
   operador / locutor). Login exchanges name+password for a bearer token +
   the user (with its resolved permissions, so the UI gates menus without
   re-deriving the matrix). The token is kept in localStorage and attached to
   authenticated calls. The password is only ever sent to /auth/login — never
   stored, never logged. The /users routes need users.manage (admin only). */

export type UserDTO = {
  id: string;
  name: string;
  role: string;
  permissions: string[];
};

export const ROLE_LABELS: Record<string, string> = {
  admin: "Administrador",
  operador: "Operador",
  locutor: "Locutor",
};

const TOKEN_KEY = "autodj.token";

/* reads the persisted bearer token ("" when logged out / SSR). */
export function getToken(): string {
  if (typeof window === "undefined") return "";
  return window.localStorage.getItem(TOKEN_KEY) ?? "";
}

/* persists (or clears when "") the bearer token. */
export function setToken(token: string): void {
  if (typeof window === "undefined") return;
  if (token) window.localStorage.setItem(TOKEN_KEY, token);
  else window.localStorage.removeItem(TOKEN_KEY);
}

/* Authorization header for the current token, or {} when logged out. */
function authHeader(): Record<string, string> {
  const t = getToken();
  return t ? { Authorization: "Bearer " + t } : {};
}

/* POST /auth/login → { token, user }. 401 = bad credentials (HTTPError.status). */
export async function login(
  name: string,
  password: string,
): Promise<{ token: string; user: UserDTO }> {
  const res = await fetch(apiBase() + "/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, password }),
  });
  if (!res.ok) await throwHuman("login", res);
  return await res.json();
}

/* GET /auth/me → the authenticated user for the stored token. 401 = no/bad
   token (surfaced so the hook can log out). */
export async function fetchMe(): Promise<UserDTO> {
  const res = await fetch(apiBase() + "/api/v1/auth/me", { headers: authHeader() });
  if (!res.ok) await throwHuman("me", res);
  return await res.json();
}

/* GET /users → every account (needs users.manage; 403 otherwise). */
export async function fetchUsers(): Promise<UserDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/users", { headers: authHeader() });
  if (!res.ok) await throwHuman("users", res);
  return (await res.json()) ?? [];
}

/* POST /users → creates an account. 409 = name taken; 422 = bad role/name. */
export async function createUser(body: {
  name: string;
  role: string;
  password: string;
}): Promise<UserDTO> {
  const res = await fetch(apiBase() + "/api/v1/users", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: JSON.stringify(body),
  });
  if (!res.ok) await throwHuman("createUser", res);
  return await res.json();
}

/* PATCH /users/{id} → changes role and (when non-empty) password. 404 = unknown. */
export async function updateUser(
  id: string,
  body: { role: string; password?: string },
): Promise<UserDTO> {
  const res = await fetch(apiBase() + "/api/v1/users/" + encodeURIComponent(id), {
    method: "PATCH",
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: JSON.stringify(body),
  });
  if (!res.ok) await throwHuman("updateUser", res);
  return await res.json();
}

/* DELETE /users/{id} → removes an account. 404 = unknown. */
export async function deleteUser(id: string): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/users/" + encodeURIComponent(id), {
    method: "DELETE",
    headers: authHeader(),
  });
  if (!res.ok && res.status !== 204) await throwHuman("deleteUser", res);
}

/* ============ Operator profile / player layout (Story 7.4) ============ */

/* one panel preference: which pane, shown or hidden, where in the order, and which
   monitor it lives on (screen: 0 principal, 1 secondary — Story 9.2). */
export type PanelPrefDTO = { key: string; visible: boolean; order: number; screen: number };

/* the operator's saved player layout. The backend serves Default() when none was
   ever saved, so this is never null for a logged-in user. touch enlarges hit
   targets; screens (1|2) is the monitor count the layout spreads across (9.2). */
export type ProfileDTO = {
  userId: string;
  theme: string; // "auto" | "claro" | "escuro"
  density: string; // "confortavel" | "compacto"
  touch: boolean;
  screens: number; // 1 | 2
  panels: PanelPrefDTO[];
};

/* GET /profile/me → the authenticated operator's layout (or the shipped default).
   401 = no/bad token. */
export async function fetchProfile(): Promise<ProfileDTO> {
  const res = await fetch(apiBase() + "/api/v1/profile/me", { headers: authHeader() });
  if (!res.ok) await throwHuman("profile", res);
  return await res.json();
}

/* PUT /profile/me → saves the layout, returns the normalized stored value. The
   owner is taken from the token server-side; any userId here is ignored. 422 =
   invalid layout; 403 = not the owner. */
export async function saveProfile(body: {
  theme: string;
  density: string;
  touch: boolean;
  screens: number;
  panels: PanelPrefDTO[];
}): Promise<ProfileDTO> {
  const res = await fetch(apiBase() + "/api/v1/profile/me", {
    method: "PUT",
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: JSON.stringify(body),
  });
  if (!res.ok) await throwHuman("saveProfile", res);
  return await res.json();
}

/* PANEL_LABELS maps the domain panel keys to their pt-BR display names, so the
   layout editor carries no hardcoded catalog beyond the label lookup. */
export const PANEL_LABELS: Record<string, string> = {
  fila: "Fila",
  biblioteca: "Biblioteca",
  cartwall: "Cartwall",
  spotify: "Spotify",
  aidj: "AI DJ",
};

/* ============ PlayLog / mais tocadas (Story 7.5) ============ */

/* one append-only history row: what actually aired. broadcasted/ecad_relevant are
   snapshotted from the two-engine matrix at play time (a DRM Spotify play → both
   false), so reports filter on them. */
export type PlayLogEntryDTO = {
  id: string;
  track_id: string;
  title: string;
  artist: string;
  source: string;
  played_at: string; // RFC3339
  duration_ms: number;
  broadcasted: boolean;
  ecad_relevant: boolean;
};

/* one row of the "most played" ranking. */
export type TopItemDTO = {
  track_id: string;
  title: string;
  artist: string;
  count: number;
};

/* GET /playlog?from&to → the play history in [from, to), oldest first. Both bounds
   are RFC3339; the caller passes a rolling window. 400 = missing/invalid bound. */
export async function fetchPlayLog(from: Date, to: Date): Promise<PlayLogEntryDTO[]> {
  const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
  const res = await fetch(apiBase() + "/api/v1/playlog?" + q.toString(), { headers: authHeader() });
  if (!res.ok) await throwHuman("playlog", res);
  return await res.json();
}

/* GET /playlog/top?period&n → the n most played tracks over the named rolling
   window. period is dia|semana|mes (invalid → 422); n defaults to 10, clamped
   [1,100] server-side. */
export async function fetchTop(period: string, n = 10): Promise<TopItemDTO[]> {
  const q = new URLSearchParams({ period, n: String(n) });
  const res = await fetch(apiBase() + "/api/v1/playlog/top?" + q.toString(), { headers: authHeader() });
  if (!res.ok) await throwHuman("playlogTop", res);
  return await res.json();
}

/* PLAYLOG_PERIODS maps the domain period keys to pt-BR labels for the ranking
   selector — no hardcoded catalog beyond the window names the backend accepts. */
export const PLAYLOG_PERIODS: { key: string; label: string }[] = [
  { key: "dia", label: "Dia" },
  { key: "semana", label: "Semana" },
  { key: "mes", label: "Mês" },
];

/* one aggregated work in the ECAD execution report: a track with its play count
   and total aired duration over the window. */
export type ECADLineDTO = {
  track_id: string;
  title: string;
  artist: string;
  plays: number;
  total_duration_ms: number;
};

/* the ECAD execution report over [from, to): the ECAD-relevant plays aggregated
   by track, ordered by plays desc. DRM Spotify never appears (ecad_relevant=false
   at play time). Totals are the sum over lines. */
export type ECADReportDTO = {
  from: string; // RFC3339
  to: string; // RFC3339
  generated_at: string; // RFC3339
  total_plays: number;
  total_duration_ms: number;
  lines: ECADLineDTO[];
};

/* GET /reports/ecad?from&to → the ECAD execution report over [from, to). Both
   bounds are RFC3339; 400 = missing/invalid bound, 422 = from >= to. The report
   is a read-only projection over the immutable play log. */
export async function fetchECADReport(from: Date, to: Date): Promise<ECADReportDTO> {
  const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
  const res = await fetch(apiBase() + "/api/v1/reports/ecad?" + q.toString(), { headers: authHeader() });
  if (!res.ok) await throwHuman("ecadReport", res);
  return await res.json();
}

/* one aired play in the broadcast proof — one line per execution, never
   aggregated. source is exposed for transparency (local/tts/line_in); DRM
   Spotify never appears (broadcasted=false at play time). */
export type ProofLineDTO = {
  entry_id: string;
  played_at: string; // RFC3339
  track_id: string;
  title: string;
  artist: string;
  source: string;
  duration_ms: number;
};

/* the chronological broadcast proof over [from, to): every aired play, in play
   order — auditable evidence of what actually went to air. Totals are the line
   count and summed durations. */
export type BroadcastProofDTO = {
  from: string; // RFC3339
  to: string; // RFC3339
  generated_at: string; // RFC3339
  total_entries: number;
  total_duration_ms: number;
  lines: ProofLineDTO[];
};

/* GET /reports/broadcast?from&to → the chronological broadcast proof over
   [from, to). Both bounds are RFC3339; 400 = missing/invalid bound, 422 = from
   >= to. Read-only projection over the immutable play log. */
export async function fetchBroadcastProof(from: Date, to: Date): Promise<BroadcastProofDTO> {
  const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
  const res = await fetch(apiBase() + "/api/v1/reports/broadcast?" + q.toString(), { headers: authHeader() });
  if (!res.ok) await throwHuman("broadcastProof", res);
  return await res.json();
}

/* a listener request / dedication in the moderation queue (Story 8.3). status is
   pending|approved|rejected|played; handled_at is empty until the first
   transition. This is metadata (song hint text), never DRM audio — the queue is
   independent of the two-engine invariant. */
export type RequestDTO = {
  id: string;
  listener_name: string;
  song_hint: string;
  dedication: string;
  status: string;
  created_at: string; // RFC3339
  handled_at?: string; // RFC3339, omitted while pending
};

/* the moderation statuses the queue accepts, with pt-BR labels for the filter —
   mirrors the domain matrix (no hardcoded list beyond what the backend validates). */
export const REQUEST_STATUSES: { key: string; label: string }[] = [
  { key: "pending", label: "Pendentes" },
  { key: "approved", label: "Aprovados" },
  { key: "rejected", label: "Rejeitados" },
  { key: "played", label: "Tocados" },
];

/* GET /requests?status → the moderation queue, optionally filtered. Empty status
   returns all; an unknown status → 422 (HTTPError.status). */
export async function fetchRequests(status = ""): Promise<RequestDTO[]> {
  const q = status ? "?" + new URLSearchParams({ status }).toString() : "";
  const res = await fetch(apiBase() + "/api/v1/requests" + q, { headers: authHeader() });
  if (!res.ok) await throwHuman("requests", res);
  return await res.json();
}

/* POST /requests → record a new pending request. Blank listener/song → 422. */
export async function submitRequest(
  listener_name: string,
  song_hint: string,
  dedication: string,
): Promise<RequestDTO> {
  const res = await fetch(apiBase() + "/api/v1/requests", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: JSON.stringify({ listener_name, song_hint, dedication }),
  });
  if (!res.ok) await throwHuman("submitRequest", res);
  return await res.json();
}

/* POST /requests/{id}/{action} → moderate a request. action is
   approve|reject|played. Unknown id → 404; forbidden transition → 409. */
export async function moderateRequest(
  id: string,
  action: "approve" | "reject" | "played",
): Promise<RequestDTO> {
  const res = await fetch(apiBase() + "/api/v1/requests/" + encodeURIComponent(id) + "/" + action, {
    method: "POST",
    headers: authHeader(),
  });
  if (!res.ok) await throwHuman("moderateRequest", res);
  return await res.json();
}

/* the track currently on the air in the public snapshot (Story 8.4). on_air is
   false when nothing plays. mixable mirrors the two-engine matrix: DRM Spotify
   is never mixable, so the badge shows the source honestly. */
export type PubNowDTO = {
  track_id: string;
  title: string;
  source: string;
  on_air: boolean;
  mixable: boolean;
};

/* one queued track in the public "vai tocar" list. */
export type PubUpcomingDTO = {
  track_id: string;
  title: string;
  source: string;
};

/* one recently aired track in the public "já tocou" list (newest first). */
export type PubPlayedDTO = {
  track_id: string;
  title: string;
  artist: string;
  source: string;
  played_at: string; // RFC3339
  duration_ms: number;
};

/* the daily fact sheet — calendar/astronomy computed purely from the date
   (weekday, moon phase/illumination, zodiac, hemisphere season, day of year,
   holidays). No external weather call — deterministic for home use. */
export type PubDailyDTO = {
  date: string; // RFC3339
  weekday: string;
  moon_phase: string;
  moon_illum: number;
  zodiac: string;
  season: string;
  day_of_year: number;
  holidays: string[];
};

/* the whole read-only public snapshot: what's on air, what's next, what just
   played, plus the daily info. A projection over playback + the play log +
   the clock — no auth, no state. */
export type PubSiteDTO = {
  generated_at: string; // RFC3339
  now_playing: PubNowDTO;
  upcoming: PubUpcomingDTO[];
  recent: PubPlayedDTO[];
  daily: PubDailyDTO;
};

/* GET /public/site → the public snapshot. Public endpoint (no auth header
   needed, but sending one is harmless). */
export async function fetchPublicSite(): Promise<PubSiteDTO> {
  const res = await fetch(apiBase() + "/api/v1/public/site", { headers: authHeader() });
  if (!res.ok) await throwHuman("publicSite", res);
  return await res.json();
}

/* ---- social integrations (Story 8.5) ---- */

/* one targetable social platform (from the API, no hardcoded list). */
export type SocPlatformDTO = {
  key: string;
  label: string;
  max_len: number;
};

/* one publish receipt returned by the gateway (stub in the home build). */
export type SocReceiptDTO = {
  platform: string;
  external_id: string;
  url: string;
  published_at: string; // RFC3339
};

/* wraps the receipts returned by an announce. */
export type SocAnnounceDTO = {
  receipts: SocReceiptDTO[];
};

/* GET /social/platforms → the platforms the UI can target. */
export async function fetchSocialPlatforms(): Promise<SocPlatformDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/social/platforms", { headers: authHeader() });
  if (!res.ok) await throwHuman("socialPlatforms", res);
  return await res.json();
}

/* POST /social/announce → publishes the current now-playing track to each
   platform. 409 if nothing is on air. */
export async function announceNowPlaying(platforms: string[]): Promise<SocAnnounceDTO> {
  const res = await fetch(apiBase() + "/api/v1/social/announce", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: JSON.stringify({ platforms }),
  });
  if (!res.ok) await throwHuman("socialAnnounce", res);
  return await res.json();
}

/* POST /social/post → publishes a free-text note to one platform. */
export async function postSocialCustom(platform: string, text: string): Promise<SocReceiptDTO> {
  const res = await fetch(apiBase() + "/api/v1/social/post", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: JSON.stringify({ platform, text }),
  });
  if (!res.ok) await throwHuman("socialPost", res);
  return await res.json();
}

/* ---- Staff schedule (Story 9.1) — the announcer roster (human grid) ---- */

/* one announcer shift; start_min/end_min are minutes since midnight [start,end). */
export type StaffShiftDTO = {
  id: string;
  announcer_id: string;
  announcer_name: string;
  weekday: number; // 0=Sun … 6=Sat
  start_min: number;
  end_min: number;
};

/* who is on air now: on_air=false → nobody scheduled (shift absent). */
export type StaffOnAirDTO = {
  on_air: boolean;
  shift?: StaffShiftDTO;
};

/* GET /staff/shifts → the roster ordered by (weekday, start). */
export async function fetchStaffShifts(): Promise<StaffShiftDTO[]> {
  const res = await fetch(apiBase() + "/api/v1/staff/shifts", { headers: authHeader() });
  if (!res.ok) await throwHuman("staffShifts", res);
  return await res.json();
}

/* POST /staff/shifts → registers a new shift. 422 validation, 409 overlap. */
export async function addStaffShift(input: {
  announcer_id: string;
  announcer_name: string;
  weekday: number;
  start_min: number;
  end_min: number;
}): Promise<StaffShiftDTO> {
  const res = await fetch(apiBase() + "/api/v1/staff/shifts", {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeader() },
    body: JSON.stringify(input),
  });
  if (!res.ok) await throwHuman("addStaffShift", res);
  return await res.json();
}

/* DELETE /staff/shifts/{id} → removes a shift. 404 if unknown. */
export async function removeStaffShift(id: string): Promise<void> {
  const res = await fetch(apiBase() + "/api/v1/staff/shifts/" + encodeURIComponent(id), {
    method: "DELETE",
    headers: authHeader(),
  });
  if (!res.ok && res.status !== 204) await throwHuman("removeStaffShift", res);
}

/* GET /staff/onair → the announcer on air right now (or on_air=false). */
export async function fetchStaffOnAir(): Promise<StaffOnAirDTO> {
  const res = await fetch(apiBase() + "/api/v1/staff/onair", { headers: authHeader() });
  if (!res.ok) await throwHuman("staffOnAir", res);
  return await res.json();
}
