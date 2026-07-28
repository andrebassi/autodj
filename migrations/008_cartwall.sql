-- 008_cartwall: operator cartwall board (Story 5.4). Single-row (one board per
-- user): CHECK(id=1) pins one row. The buttons (label/track/hue/icon/hotkey/
-- page) are stored as a JSONB blob in `data` so the schema does not churn as the
-- button shape evolves. Absence of the row means "no board" — the app layer
-- serves cartwall.Default(). Buttons reference track ids logically; the app
-- enforces the two-engine mixable guard (a button may only fire a local PCM
-- track), not a foreign key.
CREATE TABLE IF NOT EXISTS cartwall (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data JSONB   NOT NULL
);
