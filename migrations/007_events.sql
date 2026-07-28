-- 007_events: scheduled events (Story 4.4). Multi-row, id-keyed fixed-slot
-- programming that preempts the music rotation (Voz do Brasil, hora certa,
-- temperatura, comercial, programete). `kind` is denormalized for querying; the
-- full recurrence (days/hour/minute/priority/mediaRef) is stored as a JSONB blob
-- in `data` so the schema does not churn as the event shape evolves. No track
-- FKs — MediaRef is resolved (track id or media category) to a concrete local,
-- mixable/PCM track at inject time. Domain remains the source of truth for the
-- recurrence and priority invariants.
CREATE TABLE IF NOT EXISTS events (
    id   TEXT  PRIMARY KEY,
    kind TEXT  NOT NULL,
    data JSONB NOT NULL
);
