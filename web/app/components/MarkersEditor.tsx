"use client";

import { useMemo, useState } from "react";
import type { Marker } from "@/lib/uiContent";
import type { MarkerDTO } from "@/lib/api";
import type { OpMedia } from "@/lib/operational";
import { setMarkers } from "@/lib/api";
import { useWaveform } from "@/hooks/useWaveform";
import { fmtDur } from "@/lib/format";

/* singleton cue kinds — at most one per track (server enforces the same set).
   hook (refrão) and stamp (carimbo) may repeat. */
const SINGLETON = new Set(["start", "end", "intro", "mixin", "mixout"]);

/* "Marcações Especiais" — real waveform editor with cue markers.
   Server peaks (useWaveform) draw the wave; palette buttons drop a marker at the
   playhead; markers persist via PUT /markers. DRM (Spotify) tracks are read-only
   — the two-engine invariant: no PCM ⇒ no waveform, no markers. */
export function MarkersEditor({
  palette,
  media,
  onClose,
  onSaved,
}: {
  palette: Marker[];
  media: OpMedia;
  onClose: () => void;
  onSaved?: () => void;
}) {
  const pcm = media.pcm ?? media.source === "local";
  const durationMs = media.durationMs ?? 0;
  const { peaks, loading } = useWaveform(media.id, pcm);
  const [cues, setCues] = useState<MarkerDTO[]>(media.cues ?? []);
  const [head, setHead] = useState(0); // playhead fraction [0,1]
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState("");

  /* kind → presentation (color/ico/label) from the palette */
  const look = useMemo(() => {
    const m = new Map<string, Marker>();
    for (const p of palette) m.set(p.key, p);
    return m;
  }, [palette]);

  const sortedCues = useMemo(
    () => [...cues].sort((a, b) => a.pos_ms - b.pos_ms),
    [cues],
  );

  const addMarker = (kind: string) => {
    if (!pcm || durationMs <= 0) return;
    const pos = Math.round(head * durationMs);
    setCues((prev) => {
      const next = SINGLETON.has(kind) ? prev.filter((c) => c.kind !== kind) : [...prev];
      next.push({ kind, pos_ms: pos, label: "" });
      return next;
    });
  };

  const delMarker = (idx: number) => {
    const target = sortedCues[idx];
    setCues((prev) => {
      const i = prev.findIndex(
        (c) => c.kind === target.kind && c.pos_ms === target.pos_ms,
      );
      if (i < 0) return prev;
      const next = [...prev];
      next.splice(i, 1);
      return next;
    });
  };

  const onWaveClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    setHead(Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)));
  };

  const save = async () => {
    if (!media.id) return;
    setSaving(true);
    setErr("");
    try {
      await setMarkers(media.id, cues);
      onSaved?.();
      onClose();
    } catch {
      setErr("Falha ao salvar marcadores.");
    } finally {
      setSaving(false);
    }
  };

  const frac = (posMs: number) => (durationMs > 0 ? posMs / durationMs : 0);

  return (
    <div className="mk-overlay" onMouseDown={onClose}>
      <section className="mk" onMouseDown={(e) => e.stopPropagation()}>
        <header className="mk-titlebar">
          <span className="mk-ico">🎚</span>
          <span className="mk-title">AutoDJ — Marcações Especiais</span>
          <span className="mk-wbtns">
            <button className="mk-wbtn close" onClick={onClose} title="Fechar">
              ✕
            </button>
          </span>
        </header>

        {!pcm ? (
          <div className="mk-drm">
            🔒 Faixa Spotify (DRM) — sem PCM. Waveform e marcadores disponíveis
            só para arquivos locais.
          </div>
        ) : (
          <>
            <div className="mk-toolbar">
              {palette.map((m) => (
                <button
                  key={m.key}
                  className="mk-marker"
                  style={{ borderColor: m.color }}
                  onClick={() => addMarker(m.key)}
                  title={`Marcar ${m.label} na posição atual`}
                >
                  <span style={{ color: m.color }}>{m.ico}</span>
                  {m.label}
                </button>
              ))}
            </div>

            <div className="mk-wave" onClick={onWaveClick}>
              {/* réguas dos marcadores sobre a waveform */}
              {sortedCues.map((c, i) => {
                const p = look.get(c.kind);
                return (
                  <span
                    key={`${c.kind}-${c.pos_ms}-${i}`}
                    className="mk-flag"
                    style={{ left: `${frac(c.pos_ms) * 100}%`, background: p?.color ?? "#888", color: p?.color ?? "#888" }}
                    title={`${p?.label ?? c.kind} — ${fmtDur(c.pos_ms)}`}
                  />
                );
              })}
              {/* playhead */}
              <span className="mk-head" style={{ left: `${head * 100}%` }} />
              <div className="mk-bars">
                {(peaks.length ? peaks : Array(160).fill(0.04)).map((h, i) => (
                  <span key={i} className="mk-bar" style={{ height: `${h * 100}%` }} />
                ))}
              </div>
            </div>

            <div className="mk-cuelist">
              {sortedCues.length === 0 ? (
                <span className="mk-cue-empty">
                  Sem marcadores. Clique na onda p/ posicionar, depois um botão acima.
                </span>
              ) : (
                sortedCues.map((c, i) => {
                  const p = look.get(c.kind);
                  return (
                    <span key={`${c.kind}-${c.pos_ms}-${i}`} className="mk-cue" style={{ borderColor: p?.color }}>
                      <span style={{ color: p?.color }}>{p?.ico ?? "•"}</span>
                      {p?.label ?? c.kind} · {fmtDur(c.pos_ms)}
                      <button className="mk-cue-del" onClick={() => delMarker(i)} title="Remover">
                        ✕
                      </button>
                    </span>
                  );
                })
              )}
            </div>

            <div className="mk-nowline">
              Música: <b>{media.title}</b> — {media.artist}
              {loading && <span className="mk-loading"> · carregando onda…</span>}
            </div>

            <footer className="mk-footer">
              <span className="mk-elapsed">{fmtDur(head * durationMs)}</span>
              {err && <span className="mk-err">{err}</span>}
              <div className="mk-transport" />
              <button className="mk-ctl mk-save" onClick={save} disabled={saving || !media.id}>
                {saving ? "Salvando…" : "Salvar"}
              </button>
            </footer>
          </>
        )}
      </section>
    </div>
  );
}
