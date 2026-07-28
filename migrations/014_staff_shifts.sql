-- Announcer roster / staff schedule (Story 9.1). Multi-row, id-keyed human grid:
-- which announcer is on air in which weekday time slot. announcer_id and weekday
-- are denormalized for filtering/ordering; the full shift lives in the data JSON
-- blob. The overlap invariant is enforced in the domain, not the DB.
CREATE TABLE IF NOT EXISTS staff_shifts (
    id           TEXT PRIMARY KEY,
    announcer_id TEXT NOT NULL,
    weekday      INTEGER NOT NULL,
    data         TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS staff_shifts_weekday ON staff_shifts (weekday);
