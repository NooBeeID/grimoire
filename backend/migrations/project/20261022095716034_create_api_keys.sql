-- T04 project/api-key. gen_random_uuid() bawaan Postgres >= 13.
-- Hanya prefix dan SHA-256(secret) yang disimpan; plaintext key tidak pernah masuk DB.

CREATE TABLE IF NOT EXISTS api_keys (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             text        NOT NULL,
    prefix           text        NOT NULL,
    secret_hash      bytea       NOT NULL,
    created_by       uuid        NULL,  -- tanpa FK ke users (rule 8.5); label snapshot di bawah
    created_by_label text        NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_used_at     timestamptz NULL,
    revoked_at       timestamptz NULL,
    revoked_by       uuid        NULL,  -- tanpa FK (rule 8.5)
    revoked_by_label text        NULL,
    CONSTRAINT api_keys_name_len CHECK (char_length(name) BETWEEN 2 AND 60),
    CONSTRAINT api_keys_prefix_fmt CHECK (prefix ~ '^[a-z0-9]{8}$'),
    CONSTRAINT api_keys_secret_hash_len CHECK (octet_length(secret_hash) = 32),
    CONSTRAINT api_keys_revoked_consistent CHECK (revoked_at IS NOT NULL OR (revoked_by IS NULL AND revoked_by_label IS NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_prefix_key ON api_keys (prefix);
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_project_active_name_key ON api_keys (project_id, name) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS api_keys_project_idx ON api_keys (project_id, created_at DESC);
