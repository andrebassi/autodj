/* client-only loader + minimal typings for the Spotify Web Playback SDK
   (Story 6.3). The SDK plays a full Spotify track inside the browser through an
   isolated <audio>/EME pipeline that never exposes PCM — so this player is walled
   off from the local Web Audio deck/cartwall graph. DRM ⇒ no crossfade/beatmatch;
   the auto-DJ only fades the SDK's own volume + bridges with a local jingle/voice.

   Loading is guarded by `typeof window` so the Next static export/SSR never
   touches it. The SDK script fires window.onSpotifyWebPlaybackSDKReady when ready. */

export type SpotifyPlayerState = {
  paused: boolean;
  position: number; // ms
  duration: number; // ms
  trackUri: string;
  trackName: string;
};

// Shape of the subset of the SDK we use. The SDK attaches Spotify to window.
export type SpotifyWebPlayer = {
  connect(): Promise<boolean>;
  disconnect(): void;
  addListener(event: string, cb: (arg: unknown) => void): boolean;
  removeListener(event: string): boolean;
  togglePlay(): Promise<void>;
  pause(): Promise<void>;
  resume(): Promise<void>;
  seek(ms: number): Promise<void>;
  setVolume(v: number): Promise<void>;
  getCurrentState(): Promise<unknown>;
  /** Libera o elemento de áudio interno do SDK dentro de um GESTO do usuário.
   *  Existe justamente por causa da política de autoplay (obrigatório em iOS/
   *  Safari, inofensivo nos demais). Opcional: versões antigas do SDK não têm. */
  activateElement?(): Promise<void>;
};

type SpotifyGlobal = {
  Player: new (opts: {
    name: string;
    getOAuthToken: (cb: (token: string) => void) => void;
    volume?: number;
  }) => SpotifyWebPlayer;
};

declare global {
  interface Window {
    Spotify?: SpotifyGlobal;
    onSpotifyWebPlaybackSDKReady?: () => void;
  }
}

const SDK_URL = "https://sdk.scdn.co/spotify-player.js";
let loading: Promise<SpotifyGlobal> | null = null;

/* injects the SDK <script> once and resolves when window.Spotify is ready.
   Rejects on non-browser (SSR) so callers stay client-only. Memoized. */
export function loadSpotifySdk(): Promise<SpotifyGlobal> {
  if (typeof window === "undefined") {
    return Promise.reject(new Error("spotify sdk: browser only"));
  }
  if (window.Spotify) return Promise.resolve(window.Spotify);
  if (loading) return loading;

  loading = new Promise<SpotifyGlobal>((resolve, reject) => {
    window.onSpotifyWebPlaybackSDKReady = () => {
      if (window.Spotify) resolve(window.Spotify);
      else reject(new Error("spotify sdk: loaded but Spotify global missing"));
    };
    const s = document.createElement("script");
    s.src = SDK_URL;
    s.async = true;
    s.onerror = () => reject(new Error("spotify sdk: failed to load script"));
    document.body.appendChild(s);
  });
  return loading;
}

/* transfers playback of a track URI to the SDK device and starts it. Uses the
   Web API /me/player/play (needs the same user token). Kept here so the hook
   stays declarative. */
export async function playUriOnDevice(
  token: string,
  deviceId: string,
  uri: string,
): Promise<void> {
  const res = await fetch(
    "https://api.spotify.com/v1/me/player/play?device_id=" +
      encodeURIComponent(deviceId),
    {
      method: "PUT",
      headers: {
        Authorization: "Bearer " + token,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ uris: [uri] }),
    },
  );
  // 204 = playing; 202 = accepted (device waking). Anything else is an error.
  if (res.status !== 204 && res.status !== 202) {
    throw new Error("spotify play: status " + res.status);
  }
}
