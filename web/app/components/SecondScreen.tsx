"use client";

import { useNowPlaying } from "@/hooks/useNowPlaying";
import { useClock } from "@/hooks/useClock";

/* SecondScreen (Story 9.2): the dedicated wallboard for the operator's second
   monitor. Opened as a separate browser window via ?monitor=2, so it does NOT
   mirror the console DOM — it is a read-only, touch-friendly "no ar" board that
   reuses the same live WebSocket (useNowPlaying) and clock. Big now-playing plus
   the upcoming queue, nothing clickable. One component per file. */
export function SecondScreen() {
  const { snap, connected } = useNowPlaying();
  const now = useClock();

  const np = snap?.now_playing;
  const queue = snap?.queue ?? [];

  return (
    <div className="second-screen">
      <header className="second-top">
        <span className="second-brand">AutoDJ · No ar</span>
        <span className={"second-live" + (connected ? " on" : "")}>
          {connected ? "● AO VIVO" : "○ reconectando…"}
        </span>
        <span className="second-clock">
          {now
            ? now.toLocaleTimeString("pt-BR", {
                hour: "2-digit",
                minute: "2-digit",
                second: "2-digit",
              })
            : "--:--:--"}
        </span>
      </header>

      <section className="second-now">
        <div className="second-now-tag">TOCANDO AGORA</div>
        <div className="second-now-title">{np?.title ?? "—"}</div>
        <div className="second-now-src">{np ? sourceLabel(np.source) : ""}</div>
      </section>

      <section className="second-queue">
        <div className="second-queue-tag">A SEGUIR</div>
        {queue.length === 0 ? (
          <div className="second-queue-empty">Fila vazia</div>
        ) : (
          <ol className="second-queue-list">
            {queue.slice(0, 8).map((item, i) => (
              <li key={i} className="second-queue-row">
                <span className="second-queue-pos">{i + 1}</span>
                <span className="second-queue-title">{item.playable.title}</span>
                <span className="second-queue-src">
                  {sourceLabel(item.playable.source)}
                </span>
              </li>
            ))}
          </ol>
        )}
      </section>
    </div>
  );
}

/* sourceLabel turns the domain source kind into a short pt-BR badge. */
function sourceLabel(source: string): string {
  switch (source) {
    case "local":
      return "Local";
    case "spotify":
      return "Spotify";
    case "youtube":
      return "YouTube";
    case "line_in":
      return "Line-in";
    case "tts":
      return "AI DJ";
    default:
      return source;
  }
}
