-- 009_spotify_account: linked Spotify OAuth session (Story 6.1). Single-row (one
-- account per user): CHECK(id=1) pins one row. The session
-- (access/refresh tokens, expiry, scopes, product) is stored as a JSONB blob in
-- `data` so the schema does not churn as the account shape evolves. Absence of
-- the row means "not connected". This table only tracks the OAuth session — it
-- creates no Track, so the two-engine split (spotify tracks are non-mixable,
-- never enter the playout bus) is untouched here.
CREATE TABLE IF NOT EXISTS spotify_account (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data JSONB   NOT NULL
);
