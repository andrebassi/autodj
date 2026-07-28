-- Story 9.2: touch mode + dual-monitor layout. Two columns on the existing
-- profiles table: `touch` (0/1) enlarges hit targets, `screens` (1|2) is the
-- monitor count the layout spreads across. Per-panel monitor assignment
-- (PanelPref.Screen) rides inside the existing `data` JSON blob — no new column.
-- Defaults cover pre-9.2 rows: touch off, single monitor. The domain enforces
-- screens ∈ {1,2} and panel screen ∈ {0,1}; the CHECK is a cheap DB guard.
ALTER TABLE profiles ADD COLUMN touch   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE profiles ADD COLUMN screens INTEGER NOT NULL DEFAULT 1 CHECK (screens IN (1,2));
