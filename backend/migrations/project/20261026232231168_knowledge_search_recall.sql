-- T20 search recall. Forward-only dan idempoten.
-- Risiko K7: ALTER TABLE ... ADD COLUMN ... GENERATED STORED menulis ulang knowledge_files di bawah
-- ACCESS EXCLUSIVE di dalam transaksi migrasi (CREATE INDEX CONCURRENTLY tidak mungkin di runner ini).
-- Migrasi berjalan saat start sebelum server melayani; durasi diukur dan dicatat di tiket T20.

-- 1. pg_trgm wajib (koreksi typo). Gagal dengan pesan jelas bila peran tidak berizin.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
EXCEPTION WHEN insufficient_privilege THEN
    RAISE EXCEPTION 'T20: ekstensi pg_trgm wajib. Minta pemilik database/superuser menjalankan: CREATE EXTENSION pg_trgm;';
END $$;

-- 2. Agregat OR untuk tsquery (tsquery_or strict: nilai NULL dilewati). CREATE OR REPLACE idempoten dan
--    tidak bergantung pada katalog global (tes memakai satu schema per tes).
CREATE OR REPLACE AGGREGATE knowledge_tsquery_or_agg(tsquery) (SFUNC = tsquery_or, STYPE = tsquery);

-- 3. Kolom sections (diisi Go; baris lama NULL sampai job backfill) dan tsvector baru.
ALTER TABLE knowledge_files ADD COLUMN IF NOT EXISTS sections jsonb NULL;

DROP INDEX IF EXISTS knowledge_files_search_gin;
ALTER TABLE knowledge_files DROP COLUMN IF EXISTS search;
ALTER TABLE knowledge_files DROP COLUMN IF EXISTS search_stem;

ALTER TABLE knowledge_files ADD COLUMN search tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', translate(path, '/._-', '    ')) || to_tsvector('simple', title), 'A') ||
    setweight(to_tsvector('simple', coalesce(sections, '[]'::jsonb)), 'B') ||
    setweight(to_tsvector('simple', left(content, 200000)), 'D')
) STORED;

ALTER TABLE knowledge_files ADD COLUMN search_stem tsvector GENERATED ALWAYS AS (
    setweight(
        to_tsvector('indonesian', translate(path, '/._-', '    ')) || to_tsvector('english', translate(path, '/._-', '    ')) ||
        to_tsvector('indonesian', title) || to_tsvector('english', title), 'A') ||
    setweight(
        to_tsvector('indonesian', coalesce(sections, '[]'::jsonb)) || to_tsvector('english', coalesce(sections, '[]'::jsonb)), 'B') ||
    setweight(
        to_tsvector('indonesian', left(content, 200000)) || to_tsvector('english', left(content, 200000)), 'D')
) STORED;

CREATE INDEX IF NOT EXISTS knowledge_files_search_gin ON knowledge_files USING gin (search);
CREATE INDEX IF NOT EXISTS knowledge_files_search_stem_gin ON knowledge_files USING gin (search_stem);
CREATE INDEX IF NOT EXISTS knowledge_files_sections_null ON knowledge_files (id) WHERE sections IS NULL;

-- 4. Kosakata project untuk koreksi typo (dipelihara trigger; skema milik module knowledge).
CREATE TABLE IF NOT EXISTS knowledge_terms (
    project_id uuid NOT NULL,
    file_id    uuid NOT NULL REFERENCES knowledge_files(id) ON DELETE CASCADE,
    term       text NOT NULL,
    PRIMARY KEY (file_id, term)
);
CREATE INDEX IF NOT EXISTS knowledge_terms_term_trgm ON knowledge_terms USING gin (term gin_trgm_ops);
CREATE INDEX IF NOT EXISTS knowledge_terms_project_term ON knowledge_terms (project_id, term);

CREATE OR REPLACE FUNCTION knowledge_terms_refresh() RETURNS trigger AS $$
BEGIN
    DELETE FROM knowledge_terms WHERE file_id = NEW.id;
    INSERT INTO knowledge_terms (project_id, file_id, term)
    SELECT NEW.project_id, NEW.id, u.lexeme
    FROM unnest(NEW.search) u
    WHERE char_length(u.lexeme) BETWEEN 3 AND 64
      AND (u.weights && ARRAY['A', 'B']::text[] OR cardinality(u.positions) >= 2)
    ON CONFLICT DO NOTHING;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS knowledge_terms_refresh ON knowledge_files;
CREATE TRIGGER knowledge_terms_refresh
    AFTER INSERT OR UPDATE OF path, title, content, sections ON knowledge_files
    FOR EACH ROW EXECUTE FUNCTION knowledge_terms_refresh();

INSERT INTO knowledge_terms (project_id, file_id, term)
SELECT f.project_id, f.id, u.lexeme
FROM knowledge_files f, LATERAL unnest(f.search) u
WHERE char_length(u.lexeme) BETWEEN 3 AND 64
  AND (u.weights && ARRAY['A', 'B']::text[] OR cardinality(u.positions) >= 2)
ON CONFLICT DO NOTHING;
