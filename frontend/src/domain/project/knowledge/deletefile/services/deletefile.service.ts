import { del } from '../../../../../shared/services/http';

export function deleteFile(projectId: string, path: string): Promise<void> {
  return del(`/api/projects/${encodeURIComponent(projectId)}/knowledge/files`, { query: { path } });
}
