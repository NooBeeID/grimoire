-- T05 project/knowledge. Forward-only. gen_random_uuid() bawaan Postgres >= 13.
CREATE TABLE IF NOT EXISTS knowledge_files (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    path             text        NOT NULL,
    title            text        NOT NULL DEFAULT '',
    content          text        NOT NULL,
    content_hash     bytea       NOT NULL,
    size_bytes       int         NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,   -- tanpa FK ke users (rule 8.5)
    created_by_label text        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    updated_by_label text        NOT NULL,
    search           tsvector    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', translate(path, '/._-', '    ')), 'A') ||
        setweight(to_tsvector('simple', title), 'B') ||
        setweight(to_tsvector('simple', left(content, 200000)), 'D')
    ) STORED,
    CONSTRAINT knowledge_files_path_len CHECK (char_length(path) BETWEEN 1 AND 512),
    CONSTRAINT knowledge_files_hash_len CHECK (octet_length(content_hash) = 32),
    CONSTRAINT knowledge_files_size_nonneg CHECK (size_bytes >= 0),
    CONSTRAINT knowledge_files_project_id_path_key UNIQUE (project_id, path)
);

CREATE INDEX IF NOT EXISTS knowledge_files_search_gin ON knowledge_files USING gin (search);
CREATE INDEX IF NOT EXISTS knowledge_files_project_path_prefix ON knowledge_files (project_id, path text_pattern_ops);
