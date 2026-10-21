import { isApiError, put } from '../../../../../shared/services/http';
import { ConflictDetailsSchema, KnowledgeFileSchema, type ConflictCurrent, type KnowledgeFile } from '../../base';
import type { UpdateFileInput } from '../types';

export function updateFile(projectId: string, path: string, input: UpdateFileInput): Promise<KnowledgeFile> {
  return put(`/api/projects/${encodeURIComponent(projectId)}/knowledge/files`, input, { query: { path }, schema: KnowledgeFileSchema });
}

// 409 dengan details version_conflict yang valid; selain itu null.
export function parseConflict(e: unknown): ConflictCurrent | null {
  if (!isApiError(e) || e.status !== 409) return null;
  const r = ConflictDetailsSchema.safeParse(e.details);
  return r.success ? r.data.current : null;
}
