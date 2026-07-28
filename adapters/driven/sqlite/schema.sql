-- SQLite-flavored schema for the local per-user library (modernc.org/sqlite,
-- pure-Go, no daemon). Mirrors the Postgres migrations 001/002 semantically but
-- uses SQLite types (TEXT/INTEGER, booleans as INTEGER 0/1). The domain remains
-- the source of truth for invariants; these CHECKs are defense-in-depth so the
-- two-engine rule (spotify never mixable) also holds at the file level.
CREATE TABLE IF NOT EXISTS tracks (
    id            TEXT    PRIMARY KEY,
    title         TEXT    NOT NULL DEFAULT '',
    artist        TEXT    NOT NULL DEFAULT '',
    album         TEXT    NOT NULL DEFAULT '',
    category      TEXT    NOT NULL DEFAULT 'music',
    source        TEXT    NOT NULL,
    path          TEXT    NOT NULL DEFAULT '',
    spotify_uri   TEXT    NOT NULL DEFAULT '',
    youtube_uri   TEXT    NOT NULL DEFAULT '',
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    bpm           REAL    NOT NULL DEFAULT 0,
    musical_key   TEXT    NOT NULL DEFAULT '',
    waveform_ref  TEXT    NOT NULL DEFAULT '',
    loudness_lufs REAL    NOT NULL DEFAULT 0,
    instrumental_path TEXT NOT NULL DEFAULT '',
    play_count    INTEGER NOT NULL DEFAULT 0,
    pcm_available INTEGER NOT NULL DEFAULT 0,
    mixable       INTEGER NOT NULL DEFAULT 0,
    markers       TEXT    NOT NULL DEFAULT '[]',
    artwork_url   TEXT    NOT NULL DEFAULT '',
    folders       TEXT    NOT NULL DEFAULT '[]',
    created_at    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT source_valid CHECK (source IN ('local','spotify','line_in','tts','youtube')),
    CONSTRAINT spotify_never_mixable CHECK (NOT (source = 'spotify' AND mixable = 1)),
    CONSTRAINT youtube_never_mixable CHECK (NOT (source = 'youtube' AND mixable = 1)),
    CONSTRAINT category_valid CHECK (category IN
        ('music','jingle','soundtrack','effect','commercial','promo','sponsorship','extra'))
);

CREATE INDEX IF NOT EXISTS tracks_artist_title ON tracks (artist, title);
CREATE INDEX IF NOT EXISTS tracks_category     ON tracks (category);
CREATE INDEX IF NOT EXISTS tracks_mixable      ON tracks (mixable);

-- Single-row station-wide fine-mixing settings (Story 3.3). The CHECK(id=1)
-- pins one row; the value object is stored as a JSON blob so the schema does not
-- churn when mixing fields evolve. Absence of the row means "factory defaults".
CREATE TABLE IF NOT EXISTS mix_settings (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data TEXT    NOT NULL
);

-- Clocks / hour templates (Story 4.1). Multi-row, id-keyed; the ordered slots
-- are stored as a JSON blob in `data` so the schema does not churn as the slot
-- shape evolves. Programming aggregate — no track FKs (a clock is a template).
CREATE TABLE IF NOT EXISTS clocks (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    data TEXT NOT NULL
);

-- Scheduled events (Story 4.4). Multi-row, id-keyed fixed-slot programming that
-- preempts the music rotation. `kind` is denormalized for querying; the full
-- recurrence (days/hour/minute/priority/mediaRef) lives in the `data` JSON blob
-- so the schema does not churn as the event shape evolves. No track FKs —
-- MediaRef is resolved (id or category) to a concrete local track at inject time.
CREATE TABLE IF NOT EXISTS events (
    id   TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    data TEXT NOT NULL
);

-- Weekly grid (Story 4.2). Single-row (one grid per station): CHECK(id=1) pins
-- one row; the sparse (weekday,hour)→clockID cells are stored as a JSON blob in
-- `data`. Absence of the row means "no programming". Grid cells reference clock
-- ids logically (validated in the app layer against known clocks), not via FK.
CREATE TABLE IF NOT EXISTS grid (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data TEXT    NOT NULL
);

-- Cartwall board (Story 5.4). Single-row (one board per user): CHECK(id=1) pins
-- one row; the buttons (label/track/hue/icon/hotkey/page) are stored as a JSON
-- blob in `data`. Absence of the row means "no board" — the app layer serves
-- cartwall.Default(). Buttons reference track ids logically (the app enforces
-- the two-engine mixable guard), not via FK.
CREATE TABLE IF NOT EXISTS cartwall (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data TEXT    NOT NULL
);

-- Spotify OAuth account (Story 6.1). Single-row (one linked account per user):
-- CHECK(id=1) pins one row; the session (tokens/expiry/scopes/product) is a JSON
-- blob in `data`. Absence of the row means "not connected". This only tracks the
-- OAuth session — it creates no Track, so the two-engine split (spotify tracks
-- are non-mixable) is untouched here.
CREATE TABLE IF NOT EXISTS spotify_account (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data TEXT    NOT NULL
);

-- YouTube OAuth account (Story 6.5). Single-row (one linked Google account per
-- user): CHECK(id=1) pins one row; the session (tokens/expiry/scopes/channel) is a
-- JSON blob in `data`. Absence of the row means "not connected". Mirrors
-- spotify_account — tracks the OAuth session only, creating no Track, so the
-- two-engine split (youtube tracks are non-mixable) is untouched here.
CREATE TABLE IF NOT EXISTS youtube_account (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data TEXT    NOT NULL
);

-- Operator accounts / RBAC (Story 7.3). Multi-row, id-keyed. `name` is unique
-- (login handle); `role` is denormalized for querying; `pass_hash` holds the
-- argon2id PHC string (never a plaintext password). The permission matrix lives
-- in domain/identity, not here — this table only persists who exists and their
-- role. Home use keeps a handful of local operators.
CREATE TABLE IF NOT EXISTS users (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL UNIQUE,
    role      TEXT NOT NULL,
    pass_hash TEXT NOT NULL DEFAULT '',

    CONSTRAINT role_valid CHECK (role IN ('admin','operador','locutor'))
);

-- Operator player layout (Story 7.4). Multi-row, keyed by user id (logical link
-- to users.id — no FK so a layout can outlive an account edit). `theme`/`density`
-- are denormalized for querying; the ordered panel prefs (key/visible/order) are
-- stored as a JSON blob in `data`. Absence of a row means the app serves
-- operatorprofile.Default(userID). The domain owns validation/normalization.
CREATE TABLE IF NOT EXISTS profiles (
    user_id TEXT PRIMARY KEY,
    theme   TEXT NOT NULL,
    density TEXT NOT NULL,
    touch   INTEGER NOT NULL DEFAULT 0,
    screens INTEGER NOT NULL DEFAULT 1,
    data    TEXT NOT NULL DEFAULT '[]',

    CONSTRAINT theme_valid   CHECK (theme   IN ('auto','claro','escuro')),
    CONSTRAINT density_valid CHECK (density IN ('confortavel','compacto')),
    CONSTRAINT screens_valid CHECK (screens IN (1,2))
);

-- Play history (Story 7.5). APPEND-ONLY — the app never issues UPDATE or DELETE
-- against this table. It is the source of truth for ECAD reports and the
-- irradiation proof, so entries are immutable once written. `id` is a
-- deterministic (track,instant) key, making Append idempotent via
-- ON CONFLICT DO NOTHING (a retried record never double-counts). `broadcasted`
-- and `ecad_relevant` are snapshotted from the two-engine matrix at play time
-- (Spotify → both 0); reports filter on them. Title/artist are snapshotted so a
-- later catalog edit cannot rewrite what actually aired.
CREATE TABLE IF NOT EXISTS play_log (
    id            TEXT    PRIMARY KEY,
    track_id      TEXT    NOT NULL,
    title         TEXT    NOT NULL DEFAULT '',
    artist        TEXT    NOT NULL DEFAULT '',
    source        TEXT    NOT NULL,
    played_at     TEXT    NOT NULL,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    broadcasted   INTEGER NOT NULL DEFAULT 0,
    ecad_relevant INTEGER NOT NULL DEFAULT 0,

    CONSTRAINT play_log_source_valid CHECK (source IN ('local','spotify','line_in','tts','youtube'))
);

CREATE INDEX IF NOT EXISTS play_log_played_at ON play_log (played_at);
CREATE INDEX IF NOT EXISTS play_log_track     ON play_log (track_id);

-- Listener requests / dedications (Story 8.3). Multi-row, id-keyed audience
-- moderation queue. `status` and `created_at` are denormalized for filtering/
-- ordering; the full request (listener/song/dedication/handledAt) lives in the
-- `data` JSON blob so the schema does not churn. The lifecycle transition matrix
-- is enforced in the domain, not the DB.
CREATE TABLE IF NOT EXISTS requests (
    id         TEXT PRIMARY KEY,
    status     TEXT NOT NULL,
    created_at TEXT NOT NULL,
    data       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS requests_status ON requests (status);

-- Announcer roster / staff schedule (Story 9.1). Multi-row, id-keyed human grid:
-- which announcer is on air in which weekday time slot. `announcer_id` and
-- `weekday` are denormalized for filtering/ordering; the full shift (name/start/
-- end) lives in the `data` JSON blob so the schema does not churn. The overlap
-- invariant (one announcer on air at a time) is enforced in the domain, not the DB.
CREATE TABLE IF NOT EXISTS staff_shifts (
    id           TEXT PRIMARY KEY,
    announcer_id TEXT NOT NULL,
    weekday      INTEGER NOT NULL,
    data         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS staff_shifts_weekday ON staff_shifts (weekday);

-- Catálogo de pastas do operador ("rock", "anos 90"). Existe para que uma pasta
-- recém-criada e AINDA VAZIA sobreviva ao reload — as pastas em uso também são
-- derivadas do JSON `tracks.folders`, e a listagem é a união das duas fontes.
-- name_lower dá unicidade sem caixa: "Rock" e "rock" são a mesma pasta.
CREATE TABLE IF NOT EXISTS folders (
    name       TEXT NOT NULL,
    name_lower TEXT NOT NULL PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
