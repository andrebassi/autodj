-- 017_youtube: YouTube as a second embed/DRM source (Story 6.5), mirroring the
-- Spotify integration. Adds the per-track youtube_uri (video id / URL), relaxes the
-- source CHECKs on tracks and play_log to admit 'youtube', enforces the new
-- two-engine invariant (youtube ⇒ !mixable — embed audio never enters the local
-- playout bus, same rule as spotify), and adds the single-row youtube_account OAuth
-- session table. Postgres alters CHECKs in place (no table rebuild, unlike SQLite).

-- Per-track YouTube video id / URL. Empty for non-YouTube sources.
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS youtube_uri TEXT NOT NULL DEFAULT '';

-- Relax the source CHECK to accept 'youtube'.
ALTER TABLE tracks DROP CONSTRAINT IF EXISTS source_valid;
ALTER TABLE tracks ADD CONSTRAINT source_valid
    CHECK (source IN ('local','spotify','line_in','tts','youtube'));

-- youtube ⇒ !mixable: embed/DRM audio (no PCM) never enters the local mix bus.
ALTER TABLE tracks DROP CONSTRAINT IF EXISTS youtube_never_mixable;
ALTER TABLE tracks ADD CONSTRAINT youtube_never_mixable
    CHECK (NOT (source = 'youtube' AND mixable));

-- Same relax on the append-only play_log.
ALTER TABLE play_log DROP CONSTRAINT IF EXISTS play_log_source_valid;
ALTER TABLE play_log ADD CONSTRAINT play_log_source_valid
    CHECK (source IN ('local','spotify','line_in','tts','youtube'));

-- Linked Google/YouTube OAuth session. Single-row (one account per user):
-- CHECK(id=1) pins one row; the session (access/refresh tokens, expiry, scopes,
-- channel title, premium best-effort) is a JSONB blob in `data`. Absence of the
-- row means "not connected". Tracks the OAuth session only — creates no Track, so
-- the two-engine split (youtube tracks are non-mixable) is untouched here.
CREATE TABLE IF NOT EXISTS youtube_account (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data JSONB   NOT NULL
);
