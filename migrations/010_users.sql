-- 010_users: operator accounts / RBAC (Story 7.3). Multi-row, id-keyed. `name`
-- is unique (login handle); `role` is denormalized for querying; `pass_hash`
-- holds the argon2id PHC string (never a plaintext password). The permission
-- matrix lives in domain/identity, not here — this table only persists who
-- exists and their role. Home use keeps a handful of local operators.
CREATE TABLE IF NOT EXISTS users (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL UNIQUE,
    role      TEXT NOT NULL,
    pass_hash TEXT NOT NULL DEFAULT '',

    CONSTRAINT role_valid CHECK (role IN ('admin','operador','locutor'))
);
