-- gen_random_uuid() bawaan Postgres >= 13; tanpa pgcrypto.

CREATE TABLE IF NOT EXISTS projects (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text        NOT NULL,
    description      text        NOT NULL DEFAULT '',
    created_by       uuid        NULL,
    created_by_label text        NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT projects_name_len CHECK (char_length(name) BETWEEN 2 AND 100),
    CONSTRAINT projects_description_len CHECK (char_length(description) <= 1000)
);
CREATE INDEX IF NOT EXISTS projects_updated_idx ON projects (updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS project_members (
    project_id     uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id        uuid        NOT NULL,  -- tanpa FK ke users (rule 8.5); konsistensi lewat bus
    role           text        NOT NULL,
    added_by       uuid        NULL,
    added_by_label text        NULL,
    added_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT project_members_pkey PRIMARY KEY (project_id, user_id),
    CONSTRAINT project_members_role_check CHECK (role IN ('owner','member'))
);
CREATE INDEX IF NOT EXISTS project_members_user_idx ON project_members (user_id);
