import { get } from '../../../../../shared/services/http';
import { KnowledgeFileSchema, type KnowledgeFile } from '../../base';

export function getFile(projectId: string, path: string, signal?: AbortSignal): Promise<KnowledgeFile> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/knowledge/files`, {
    query: { path },
    schema: KnowledgeFileSchema,
    signal,
  });
}
