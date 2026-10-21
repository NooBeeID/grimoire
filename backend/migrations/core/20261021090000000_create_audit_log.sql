CREATE TABLE IF NOT EXISTS audit_log (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    occurred_at   timestamptz NOT NULL DEFAULT now(),
    actor_type    text        NOT NULL CHECK (actor_type IN ('user','api_key','anonymous','system')),
    actor_id      uuid        NULL,
    actor_label   text        NOT NULL,
    project_id    uuid        NULL,
    project_label text        NULL,
    action        text        NOT NULL,
    target_type   text        NULL,
    target        text        NULL,
    args          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    status        text        NOT NULL CHECK (status IN ('ok','error','denied')),
    result        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    ip            inet        NULL,
    user_agent    text        NULL,
    trace_id      text        NOT NULL DEFAULT ''
);

-- id sebagai tie-breaker keyset (occurred_at DESC, id DESC).
CREATE INDEX IF NOT EXISTS audit_log_occurred_idx ON audit_log (occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_log_project_idx  ON audit_log (project_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_log_actor_idx    ON audit_log (actor_type, actor_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS audit_log_action_idx   ON audit_log (action, occurred_at DESC, id DESC);

-- Append-only dari sisi DB: UPDATE selalu ditolak. DELETE diizinkan (hanya job purge).
CREATE OR REPLACE FUNCTION audit_log_block_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$;

DROP TRIGGER IF EXISTS audit_log_no_update ON audit_log;
CREATE TRIGGER audit_log_no_update
    BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_block_update();
