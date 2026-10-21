import { post } from '../../../../../shared/services/http';
import type { MoveFileInput } from '../types';
import { KnowledgeFileSchema, type KnowledgeFile } from '../../base';

export function moveFile(projectId: string, body: MoveFileInput): Promise<KnowledgeFile> {
  return post(`/api/projects/${encodeURIComponent(projectId)}/knowledge/files/move`, body, { schema: KnowledgeFileSchema });
}
