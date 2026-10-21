import { post } from '../../../../../shared/services/http';
import { KnowledgeFileSchema, type KnowledgeFile } from '../../base';

export function createFile(projectId: string, input: { path: string; content: string }): Promise<KnowledgeFile> {
  return post(`/api/projects/${encodeURIComponent(projectId)}/knowledge/files`, input, { schema: KnowledgeFileSchema });
}
