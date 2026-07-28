-- Listener requests / dedications (Story 8.3). Multi-row, id-keyed audience
-- moderation queue. status and created_at are denormalized for filtering and
-- ordering; the full request (listener/song/dedication/handledAt) lives in the
-- JSONB data blob so the schema does not churn as the shape evolves. The
-- lifecycle transition matrix is enforced in the domain, not the DB.
CREATE TABLE IF NOT EXISTS requests (
    id         TEXT  PRIMARY KEY,
    status     TEXT  NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    data       JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS requests_status ON requests (status);
