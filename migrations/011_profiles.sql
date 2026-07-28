-- 011_profiles: operator player layout (Story 7.4). Multi-row, keyed by user id
-- (logical link to users.id — no FK so a layout can outlive an account edit).
-- `theme`/`density` are denormalized for querying; the ordered panel prefs
-- (key/visible/order) are stored as a JSONB blob in `data`. Absence of a row means
-- the app serves operatorprofile.Default(userID). The domain owns validation and
-- order normalization; these CHECKs are defense-in-depth.
CREATE TABLE IF NOT EXISTS profiles (
    user_id TEXT  PRIMARY KEY,
    theme   TEXT  NOT NULL,
    density TEXT  NOT NULL,
    data    JSONB NOT NULL DEFAULT '[]'::jsonb,

    CONSTRAINT theme_valid   CHECK (theme   IN ('auto','claro','escuro')),
    CONSTRAINT density_valid CHECK (density IN ('confortavel','compacto'))
);
