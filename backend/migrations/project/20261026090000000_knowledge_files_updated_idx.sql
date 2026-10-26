-- T18: list-changes (project.knowledge.list-changes) urut updated_at per project.
CREATE INDEX IF NOT EXISTS knowledge_files_project_updated_idx ON knowledge_files (project_id, updated_at DESC);
