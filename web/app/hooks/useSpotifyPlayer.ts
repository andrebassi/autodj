"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchSpotifyToken } from "@/lib/api";
import {
  loadSpotifySdk,
  playUriOnDevice,
  type SpotifyWebPlayer,
} from "@/lib/spotifySdk";

export type SpotifyPlayerStatus = {
  ready: boolean; // device created + connected
  playing: boolean;
  currentUri: string | null; // uri now loaded on the SDK device
  position: number; // ms
  duration: number; // ms
  error: string | null;
  // fim natural da faixa: nonce++ a cada término; endedUri = a que terminou. O
  // playout observa o nonce p/ avançar a fila (pos não serve: SDK reseta a 0).
  endedNonce: number;
  endedUri: string | null;
  // handoff de device: o Spotify começou a tocar em OUTRO aparelho (mesma conta)
  // e este device deixou de ser o ativo → SDK dispara player_state_changed(null).
  // nonce++ nesse evento (só se estávamos tocando aqui). O playout observa e dá
  // stop na estação automaticamente — não competir com o outro device.
  takeoverNonce: number;
};

const EMPTY: SpotifyPlayerStatus = {
  ready: false,
  playing: false,
  currentUri: null,
  position: 0,
  duration: 0,
  error: null,
  endedNonce: 0,
  endedUri: null,
  takeoverNonce: 0,
};

/* isolated Spotify Web Playback SDK controller (Story 6.3). Creates an "AutoDJ"
   device that plays a full Spotify track *inside the browser* — a walled-off
   EME/DRM pipeline with no PCM, so nothing here connects to the local Web Audio
   deck/cartwall or the Icecast bus. Transitions the auto-DJ makes use only this
   player's own volume fade + a local jingle/voice bridge (6.4), never a PCM mix.

   Gated by `premium`: without Premium the SDK refuses playback, so we don't even
   instantiate — the panel shows "requer Premium". The getOAuthToken callback
   pulls a fresh token from the backend (auto-refresh). */
export function useSpotifyPlayer(
  premium: boolean,
  name = "AutoDJ",
): {
  status: SpotifyPlayerStatus;
  play: (uri: string) => Promise<void>;
  toggle: () => Promise<void>;
  stop: () => Promise<void>;
  seek: (ms: number) => Promise<void>;
  setVolume: (v: number) => Promise<void>;
} {
  const [status, setStatus] = useState<SpotifyPlayerStatus>(EMPTY);
  const playerRef = useRef<SpotifyWebPlayer | null>(null);
  const deviceRef = useRef<string | null>(null);
  const tokenRef = useRef<string>("");
  // último estado do SDK p/ detectar fim natural (tocando→pausado em pos 0).
  const prevStateRef = useRef<{ paused: boolean; uri: string } | null>(null);

  useEffect(() => {
    if (!premium) {
      // tear down if premium was lost
      playerRef.current?.disconnect();
      playerRef.current = null;
      deviceRef.current = null;
      setStatus(EMPTY);
      return;
    }
    let cancelled = false;

    const boot = async () => {
      try {
        const Spotify = await loadSpotifySdk();
        if (cancelled) return;
        const player = new Spotify.Player({
          name,
          volume: 0.8,
          getOAuthToken: (cb) => {
            fetchSpotifyToken()
              .then((tok) => {
                tokenRef.current = tok;
                cb(tok);
              })
              .catch(() =>
                setStatus((s) => ({ ...s, error: "sem token Spotify" })),
              );
          },
        });

        player.addListener("ready", (arg) => {
          const id = (arg as { device_id: string }).device_id;
          deviceRef.current = id;
          setStatus((s) => ({ ...s, ready: true, error: null }));
        });
        player.addListener("not_ready", () => {
          setStatus((s) => ({ ...s, ready: false }));
        });
        player.addListener("player_state_changed", (arg) => {
          if (!arg) {
            // estado null = este device não é mais o ativo (playback foi p/ outro
            // aparelho da conta). Se estávamos tocando aqui, sinaliza takeover p/ o
            // playout parar a estação. Zera prevState p/ não disparar de novo.
            const wasPlaying = !!prevStateRef.current && !prevStateRef.current.paused;
            prevStateRef.current = null;
            if (wasPlaying) {
              setStatus((s) => ({
                ...s,
                playing: false,
                takeoverNonce: s.takeoverNonce + 1,
              }));
            }
            return;
          }
          const st = arg as {
            paused: boolean;
            position: number;
            duration: number;
            track_window: { current_track: { uri: string } };
          };
          const uri = st.track_window?.current_track?.uri ?? "";
          // fim natural: SDK vai de tocando→pausado com position 0 na MESMA faixa.
          const prev = prevStateRef.current;
          const ended =
            !!prev && !prev.paused && st.paused && st.position === 0 && prev.uri === uri;
          prevStateRef.current = { paused: st.paused, uri };
          setStatus((s) => ({
            ...s,
            playing: !st.paused,
            position: st.position,
            duration: st.duration,
            currentUri: uri || s.currentUri,
            ...(ended
              ? { endedNonce: s.endedNonce + 1, endedUri: uri || s.currentUri }
              : {}),
          }));
        });
        const initErr = (arg: unknown) =>
          setStatus((s) => ({
            ...s,
            error: (arg as { message?: string })?.message ?? "erro no player",
          }));
        player.addListener("initialization_error", initErr);
        player.addListener("authentication_error", initErr);
        player.addListener("account_error", initErr);

        await player.connect();
        if (cancelled) {
          player.disconnect();
          return;
        }
        playerRef.current = player;
        /* Política de autoplay no SDK: o elemento de áudio interno também nasce
           travado numa página recém-carregada. `activateElement()` só vale DENTRO
           de um gesto — então fica pendurado no primeiro clique/tecla do documento
           (once), igual ao destravamento dos motores local e YouTube. Sem isso, o
           refresh podia deixar o Spotify "tocando" no device e mudo aqui. */
        const activate = () => {
          void playerRef.current?.activateElement?.().catch(() => {});
        };
        const opts = { once: true, capture: true } as const;
        window.addEventListener("pointerdown", activate, opts);
        window.addEventListener("keydown", activate, opts);
      } catch (e) {
        if (!cancelled) {
          setStatus((s) => ({
            ...s,
            error: e instanceof Error ? e.message : "erro ao carregar SDK",
          }));
        }
      }
    };
    void boot();

    return () => {
      cancelled = true;
      playerRef.current?.disconnect();
      playerRef.current = null;
      deviceRef.current = null;
      prevStateRef.current = null;
    };
  }, [premium, name]);

  const play = useCallback(async (uri: string): Promise<void> => {
    const device = deviceRef.current;
    if (!device) {
      setStatus((s) => ({ ...s, error: "device Spotify não pronto" }));
      return;
    }
    try {
      const tok = tokenRef.current || (await fetchSpotifyToken());
      tokenRef.current = tok;
      await playUriOnDevice(tok, device, uri);
      setStatus((s) => ({ ...s, currentUri: uri, error: null }));
    } catch (e) {
      setStatus((s) => ({
        ...s,
        error: e instanceof Error ? e.message : "erro ao tocar",
      }));
    }
  }, []);

  const toggle = useCallback(async (): Promise<void> => {
    await playerRef.current?.togglePlay();
  }, []);

  const stop = useCallback(async (): Promise<void> => {
    await playerRef.current?.pause();
    setStatus((s) => ({ ...s, playing: false }));
  }, []);

  // seek absoluto (ms) no device do SDK — usado pelo deck p/ retomar posição no
  // refresh e p/ o seek do waveform. Atualiza a posição local otimista.
  const seek = useCallback(async (ms: number): Promise<void> => {
    const pos = Math.max(0, Math.round(ms));
    await playerRef.current?.seek(pos);
    setStatus((s) => ({ ...s, position: pos }));
  }, []);

  // volume 0..1 no device do SDK — base do fade-in/out do playout (transições
  // suaves em PRÓXIMA/RETROCEDER). É ajuste de ganho local do SDK (sem rede).
  const setVolume = useCallback(async (v: number): Promise<void> => {
    await playerRef.current?.setVolume(Math.max(0, Math.min(1, v)));
  }, []);

  return { status, play, toggle, stop, seek, setVolume };
}
