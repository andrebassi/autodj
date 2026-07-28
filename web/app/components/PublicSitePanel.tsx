"use client";

import { usePublicSite } from "@/hooks/usePublicSite";
import type { PubPlayedDTO, PubUpcomingDTO } from "@/lib/api";

/* PublicSitePanel — the read-only public site preview (Story 8.4). Shows what the
   station exposes to listeners: what's on the air now, what plays next, what just
   played (newest first), plus the daily fact sheet (weekday, moon, zodiac, season,
   holidays). It's a projection over playback + the play log + the clock — no auth,
   no state. Auto-refreshes so it tracks the air. DRM Spotify tracks appear too
   (they still play), badged honestly by source. Data from GET /public/site. */
export function PublicSitePanel({ onClose }: { onClose: () => void }) {
  const { snapshot, loaded, busy, available, error, reload } = usePublicSite();

  return (
    <div className="mix-overlay" onClick={onClose}>
      <div className="mix-modal" onClick={(e) => e.stopPropagation()}>
        <header className="mix-titlebar">AutoDJ — Site público</header>

        <div className="mix-body">
          {!loaded && <p className="mix-lbl">Carregando…</p>}
          {loaded && !available && (
            <p className="mix-lbl">Site público indisponível (endpoint não configurado).</p>
          )}
          {loaded && available && snapshot && (
            <>
              <fieldset className="mix-group">
                <legend>No ar agora</legend>
                {snapshot.now_playing.on_air ? (
                  <div className="pub-now">
                    <span className="pub-title">{snapshot.now_playing.title}</span>
                    <SourceBadge source={snapshot.now_playing.source} mixable={snapshot.now_playing.mixable} />
                  </div>
                ) : (
                  <p className="mix-lbl">Nada tocando.</p>
                )}
              </fieldset>

              <fieldset className="mix-group">
                <legend>Vai tocar</legend>
                {snapshot.upcoming.length === 0 ? (
                  <p className="mix-lbl">Fila vazia.</p>
                ) : (
                  <ul className="pub-list">
                    {snapshot.upcoming.map((u: PubUpcomingDTO, i) => (
                      <li className="pub-row" key={u.track_id + "-" + i}>
                        <span className="pub-title">{u.title}</span>
                        <SourceBadge source={u.source} />
                      </li>
                    ))}
                  </ul>
                )}
              </fieldset>

              <fieldset className="mix-group">
                <legend>Já tocou (24h)</legend>
                {snapshot.recent.length === 0 ? (
                  <p className="mix-lbl">Sem histórico recente.</p>
                ) : (
                  <ul className="pub-list">
                    {snapshot.recent.map((p: PubPlayedDTO, i) => (
                      <li className="pub-row" key={p.track_id + "-" + i}>
                        <span className="pub-time mono">{fmtTime(p.played_at)}</span>
                        <span className="pub-title">{p.title}</span>
                        <span className="pub-artist">— {p.artist}</span>
                        <SourceBadge source={p.source} />
                      </li>
                    ))}
                  </ul>
                )}
              </fieldset>

              <DailyBlock daily={snapshot.daily} />
            </>
          )}
        </div>

        <footer className="mix-footer">
          {error && <span className="mix-err">{error}</span>}
          <button className="mix-save" onClick={() => void reload()} disabled={busy}>
            Atualizar
          </button>
          <button className="mix-close" onClick={onClose}>
            Fechar
          </button>
        </footer>
      </div>
    </div>
  );
}

/* SourceBadge — honest source label. DRM Spotify shows a "sem mix" hint so the
   two-engine rule is visible to the operator previewing the public view. */
function SourceBadge({ source, mixable }: { source: string; mixable?: boolean }) {
  return (
    <span className={"pub-badge pub-src-" + source}>
      {source}
      {mixable === false ? " · sem mix" : ""}
    </span>
  );
}

/* DailyBlock — the daily fact sheet. */
function DailyBlock({ daily }: { daily: import("@/lib/api").PubDailyDTO }) {
  return (
    <fieldset className="mix-group">
      <legend>Info do dia</legend>
      <div className="pub-daily">
        <span className="pub-fact">
          <b>Dia</b> {daily.weekday} · {daily.day_of_year}º do ano
        </span>
        <span className="pub-fact">
          <b>Lua</b> {daily.moon_phase} ({daily.moon_illum}%)
        </span>
        <span className="pub-fact">
          <b>Signo</b> {daily.zodiac}
        </span>
        <span className="pub-fact">
          <b>Estação</b> {daily.season}
        </span>
        {daily.holidays.length > 0 && (
          <span className="pub-fact">
            <b>Feriado</b> {daily.holidays.join(", ")}
          </span>
        )}
      </div>
    </fieldset>
  );
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "--:--";
  return d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
}
