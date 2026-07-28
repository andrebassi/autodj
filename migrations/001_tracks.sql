-- 001_tracks: catalog table. mixable / pcm_available are the load-bearing
-- two-engine flags — the app derives them from source, never a client. A CHECK
-- enforces the spotify ⇒ !mixable invariant (no DRM audio in the local playout
-- bus) at the DB too.
CREATE TABLE IF NOT EXISTS tracks (
    id                TEXT PRIMARY KEY,
    title             TEXT NOT NULL DEFAULT '',
    artist            TEXT NOT NULL DEFAULT '',
    album             TEXT NOT NULL DEFAULT '',
    category          TEXT NOT NULL DEFAULT 'music',
    source            TEXT NOT NULL,
    path              TEXT NOT NULL DEFAULT '',
    spotify_uri       TEXT NOT NULL DEFAULT '',
    duration_ms       BIGINT NOT NULL DEFAULT 0,
    bpm               DOUBLE PRECISION NOT NULL DEFAULT 0,
    musical_key       TEXT NOT NULL DEFAULT '',
    waveform_ref      TEXT NOT NULL DEFAULT '',
    play_count        BIGINT NOT NULL DEFAULT 0,
    pcm_available     BOOLEAN NOT NULL DEFAULT FALSE,
    mixable           BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT source_valid CHECK (source IN ('local','spotify','line_in','tts')),
    CONSTRAINT spotify_never_mixable CHECK (NOT (source = 'spotify' AND mixable))
);

CREATE INDEX IF NOT EXISTS tracks_artist_title_idx ON tracks (artist, title);
CREATE INDEX IF NOT EXISTS tracks_category_idx ON tracks (category);
CREATE INDEX IF NOT EXISTS tracks_mixable_idx ON tracks (mixable);
