-- 003_markers: cue markers per track (start/intro/hook/stamp/mixin/mixout/end).
-- Stored as a JSONB array on the track — intrinsic to the track, few per row,
-- always read with it. The domain (Track.SetMarkers) is the source of truth for
-- validity (PCM-only, kind/range/singleton/order); this column just persists.
ALTER TABLE tracks ADD COLUMN IF NOT EXISTS markers JSONB NOT NULL DEFAULT '[]'::jsonb;
