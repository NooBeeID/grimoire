CREATE EXTENSION IF NOT EXISTS citext;
-- gen_random_uuid() bawaan Postgres >= 13; tanpa pgcrypto.

CREATE TABLE users (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email                citext      NOT NULL,
    name                 text        NOT NULL,
    password_hash        text        NOT NULL,
    role                 text        NOT NULL,
    is_active            boolean     NOT NULL DEFAULT true,
    must_change_password boolean     NOT NULL DEFAULT true,
    created_at           timestamptz NOT NULL DEFAULT now(),
    created_by           uuid        NULL,
    created_by_label     text        NULL,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    last_login_at        timestamptz NULL,
    CONSTRAINT users_email_key UNIQUE (email),
    CONSTRAINT users_role_check CHECK (role IN ('super_admin','developer')),
    CONSTRAINT users_name_len CHECK (char_length(name) BETWEEN 1 AND 100)
);
CREATE INDEX users_role_active_idx ON users (role, is_active);

CREATE TABLE sessions (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   bytea       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    ip           inet        NULL,
    user_agent   text        NULL,
    CONSTRAINT sessions_token_hash_key UNIQUE (token_hash)
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_last_seen_at_idx ON sessions (last_seen_at);

CREATE TABLE login_throttles (
    email             citext      PRIMARY KEY,
    failed_count      integer     NOT NULL DEFAULT 0,
    window_started_at timestamptz NOT NULL,
    locked_until      timestamptz NULL,
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_throttles_window_idx ON login_throttles (window_started_at);
