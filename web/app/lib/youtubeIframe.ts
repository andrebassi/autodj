/* client-only loader + minimal typings for the YouTube IFrame Player API
   (Story 6.7). Unlike Spotify (a remote device, no visible element) or the local
   deck (a detached <audio>), the YouTube player REQUIRES a visible DOM element in
   the page — it renders an <iframe> in place of a host <div>. That is the ONE
   structural divergence from the Spotify mirror: the deck cover slot hosts the
   player. Video is embed/DRM ⇒ no PCM ⇒ non-mixable, never touches the local Web
   Audio graph; the auto-DJ only fades the IFrame's own volume.

   Loading is guarded by `typeof window` so the Next static export/SSR never touches
   it. The script fires window.onYouTubeIframeAPIReady when window.YT is ready. */

// State constants from the IFrame API (YT.PlayerState). Kept as literals so we
// don't depend on the runtime enum before it loads.
export const YT_UNSTARTED = -1;
export const YT_ENDED = 0;
export const YT_PLAYING = 1;
export const YT_PAUSED = 2;
export const YT_BUFFERING = 3;
export const YT_CUED = 5;

// Shape of the subset of the IFrame Player we use.
export type YouTubePlayer = {
  // duas formas, ambas oficiais: id solto, ou objeto com `startSeconds` — é o
  // objeto que permite JÁ CARREGAR posicionado (retomada pós-refresh), sem tocar
  // um instante do início antes de pular.
  loadVideoById(id: string | { videoId: string; startSeconds?: number }): void;
  cueVideoById(id: string, startSeconds?: number): void;
  playVideo(): void;
  pauseVideo(): void;
  stopVideo(): void;
  seekTo(seconds: number, allowSeekAhead: boolean): void;
  setVolume(v: number): void; // 0..100
  getVolume(): number;
  // mute/unMute são a chave da PRÉ-CARGA (v0.2.11): o player engatilhado baixa o
  // vídeo MUDO (autoplay mudo é sempre permitido pelo browser, com som não) e só
  // desmuta ao entrar no ar. Distinto de setVolume(0) — a política de autoplay
  // olha o flag de mute, não o volume.
  mute(): void;
  unMute(): void;
  isMuted(): boolean;
  getCurrentTime(): number; // seconds
  getDuration(): number; // seconds
  getPlayerState(): number;
  // getVideoData().errorCode reveals embed blocks the onError event misses. For
  // "- Topic" / owner-restricted videos the player loads metadata (real title +
  // duration) but errorCode === "auth" and it never leaves UNSTARTED — no audio,
  // no onError. Polling this is the only way to detect the silent stall.
  getVideoData(): { video_id?: string; title?: string; errorCode?: string };
  destroy(): void;
};

type PlayerEvent = { target: YouTubePlayer; data: number };

type YTGlobal = {
  Player: new (
    el: HTMLElement | string,
    opts: {
      width?: number | string;
      height?: number | string;
      videoId?: string;
      host?: string;
      playerVars?: Record<string, number | string>;
      events?: {
        onReady?: (e: PlayerEvent) => void;
        onStateChange?: (e: PlayerEvent) => void;
        onError?: (e: PlayerEvent) => void;
      };
    },
  ) => YouTubePlayer;
  PlayerState: {
    UNSTARTED: number;
    ENDED: number;
    PLAYING: number;
    PAUSED: number;
    BUFFERING: number;
    CUED: number;
  };
};

declare global {
  interface Window {
    YT?: YTGlobal;
    onYouTubeIframeAPIReady?: () => void;
  }
}

const API_URL = "https://www.youtube.com/iframe_api";
let loading: Promise<YTGlobal> | null = null;

/* injects the IFrame API <script> once and resolves when window.YT is ready.
   Rejects on non-browser (SSR) so callers stay client-only. Memoized. The API
   fires window.onYouTubeIframeAPIReady after the script loads window.YT. */
export function loadYouTubeIframe(): Promise<YTGlobal> {
  if (typeof window === "undefined") {
    return Promise.reject(new Error("youtube iframe: browser only"));
  }
  // window.YT may exist but not yet have Player wired; require Player.
  if (window.YT && window.YT.Player) return Promise.resolve(window.YT);
  if (loading) return loading;

  loading = new Promise<YTGlobal>((resolve, reject) => {
    // chain any pre-existing ready handler (defensive — usually none).
    const prev = window.onYouTubeIframeAPIReady;
    window.onYouTubeIframeAPIReady = () => {
      prev?.();
      if (window.YT && window.YT.Player) resolve(window.YT);
      else reject(new Error("youtube iframe: loaded but YT.Player missing"));
    };
    const s = document.createElement("script");
    s.src = API_URL;
    s.async = true;
    s.onerror = () => reject(new Error("youtube iframe: failed to load script"));
    document.body.appendChild(s);
  });
  return loading;
}

/* extracts the bare 11-char video id from a youtube URI/URL. Accepts a raw id,
   a `youtube:video:<id>` scheme (our catalog URI), a watch URL, a youtu.be short
   URL, or an embed URL. Returns "" when nothing matches. */
export function youtubeVideoId(uri: string): string {
  if (!uri) return "";
  // our catalog scheme: youtube:video:<id> (playlist uris carry list=…, not here)
  const scheme = uri.match(/^youtube:video:([A-Za-z0-9_-]{11})$/);
  if (scheme) return scheme[1];
  // bare id
  if (/^[A-Za-z0-9_-]{11}$/.test(uri)) return uri;
  // watch?v=, youtu.be/, embed/
  const m = uri.match(/(?:v=|youtu\.be\/|embed\/)([A-Za-z0-9_-]{11})/);
  return m ? m[1] : "";
}
