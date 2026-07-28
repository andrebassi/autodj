-- 005_clocks: clocks / hour templates (Story 4.1). Multi-row, id-keyed. The
-- ordered slots are stored as a JSONB blob in `data` so the schema does not
-- churn as the slot shape evolves. A clock is a programming template — no track
-- FKs (it references categories/kinds, resolved to concrete tracks at
-- generation time). Domain remains the source of truth for slot invariants.
CREATE TABLE IF NOT EXISTS clocks (
    id   TEXT  PRIMARY KEY,
    name TEXT  NOT NULL,
    data JSONB NOT NULL
);
