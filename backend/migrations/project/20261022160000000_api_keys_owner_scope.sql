-- T13: kepemilikan key per user per project. Memperlonggar unique, tidak bisa gagal pada data T04.
-- Nama index dipertahankan agar store.ConstraintActiveName dan pemetaan ErrNameTaken tetap (K6).
-- created_by NULL dianggap berbeda satu sama lain oleh Postgres; dapat diterima (K4).
DROP INDEX IF EXISTS api_keys_project_active_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_project_active_name_key
    ON api_keys (project_id, created_by, name) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS api_keys_owner_idx ON api_keys (project_id, created_by, created_at DESC);
