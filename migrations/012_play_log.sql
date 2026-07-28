-- 012_play_log: append-only play history (Story 7.5). The app never issues
-- UPDATE or DELETE against this table — entries are immutable once written. It is
-- the single source of truth for ECAD reports and the irradiation proof. `id` is
-- a deterministic (track,instant) key, making Append idempotent via
-- ON CONFLICT DO NOTHING (a retried record never double-counts). `broadcasted`
-- and `ecad_relevant` are snapshotted from the two-engine capability matrix at
-- play time (Spotify → both false); reports filter on them. Title/artist are
-- snapshotted so a later catalog edit cannot rewrite what actually aired.
CREATE TABLE IF NOT EXISTS play_log (
    id            TEXT        PRIMARY KEY,
    track_id      TEXT        NOT NULL,
    title         TEXT        NOT NULL DEFAULT '',
    artist        TEXT        NOT NULL DEFAULT '',
    source        TEXT        NOT NULL,
    played_at     TIMESTAMPTZ NOT NULL,
    duration_ms   BIGINT      NOT NULL DEFAULT 0,
    broadcasted   BOOLEAN     NOT NULL DEFAULT FALSE,
    ecad_relevant BOOLEAN     NOT NULL DEFAULT FALSE,

    CONSTRAINT play_log_source_valid CHECK (source IN ('local','spotify','line_in','tts'))
);

CREATE INDEX IF NOT EXISTS play_log_played_at ON play_log (played_at);
CREATE INDEX IF NOT EXISTS play_log_track     ON play_log (track_id);
