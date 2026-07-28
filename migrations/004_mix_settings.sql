-- 004_mix_settings: single-row station-wide fine-mixing settings (Story 3.3).
-- The CHECK(id=1) pins exactly one row; the mixing value object is stored as a
-- JSONB blob so the schema does not churn as mixing fields evolve. Absence of
-- the row means "factory defaults" (the app returns mixing.Default()).
CREATE TABLE IF NOT EXISTS mix_settings (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data JSONB   NOT NULL
);
