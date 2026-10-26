-- T20 project/searchsynonym: kamus sinonim pencarian per project. Forward-only, idempoten.
-- Satu baris = satu kelompok setara (2..10 term huruf kecil). Keunikan term per project dijaga store
-- (advisory lock per project), bukan constraint, karena array term.
CREATE TABLE IF NOT EXISTS knowledge_synonym_groups (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    terms            text[]      NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,   -- tanpa FK ke users (rule 8.5)
    created_by_label text        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    updated_by_label text        NOT NULL,
    CONSTRAINT knowledge_synonym_groups_terms_card CHECK (cardinality(terms) BETWEEN 2 AND 10)
);

CREATE INDEX IF NOT EXISTS knowledge_synonym_groups_project_idx ON knowledge_synonym_groups (project_id, created_at, id);
CREATE INDEX IF NOT EXISTS knowledge_synonym_groups_terms_gin ON knowledge_synonym_groups USING gin (terms);
