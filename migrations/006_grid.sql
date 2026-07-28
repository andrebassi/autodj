-- 006_grid: weekly programming grid (Story 4.2). Single-row (one grid per
-- station): CHECK(id=1) pins one row. The sparse (weekday,hour)→clockID cells
-- are stored as a JSONB blob in `data` so the schema does not churn as the grid
-- shape evolves. Absence of the row means "no programming". Cells reference
-- clock ids logically (validated in the app layer against known clocks), not via
-- a foreign key — the grid is a template layer above clocks.
CREATE TABLE IF NOT EXISTS grid (
    id   INTEGER PRIMARY KEY CHECK (id = 1),
    data JSONB   NOT NULL
);
