-- T14 project/gitsync. Forward-only. Tidak menyimpan PAT.
-- last_synced_by tanpa FK ke users (rule 8.5); label snapshot (rule 8.7).
CREATE TABLE IF NOT EXISTS git_sync_configs (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id           uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind                 text        NOT NULL,
    provider             text        NOT NULL,
    host                 text        NOT NULL,
    repo_url             text        NOT NULL,
    repo_path            text        NOT NULL,
    branch               text        NOT NULL,
    folder               text        NOT NULL DEFAULT '',
    target_prefix        text        NOT NULL DEFAULT '',
    last_status          text        NULL,
    last_synced_at       timestamptz NULL,
    last_synced_by       uuid        NULL,
    last_synced_by_label text        NULL,
    last_result          jsonb       NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT git_sync_configs_kind_check CHECK (kind IN ('knowledge','api_spec')),
    CONSTRAINT git_sync_configs_provider_check CHECK (provider IN ('github','gitlab')),
    CONSTRAINT git_sync_configs_repo_url_len CHECK (char_length(repo_url) <= 2048),
    CONSTRAINT git_sync_configs_branch_len CHECK (char_length(branch) BETWEEN 1 AND 255),
    CONSTRAINT git_sync_configs_status_check CHECK (last_status IS NULL OR last_status IN ('success','partial','failed')),
    CONSTRAINT git_sync_configs_prefix_kind CHECK (kind = 'knowledge' OR target_prefix = ''),
    CONSTRAINT git_sync_configs_project_kind_key UNIQUE (project_id, kind)
);
