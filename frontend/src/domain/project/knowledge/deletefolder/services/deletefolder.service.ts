import { del } from '../../../../../shared/services/http';
import { DeleteFolderResultSchema, type DeleteFolderResult } from '../types';

export function deleteFolder(projectId: string, path: string): Promise<DeleteFolderResult> {
  if (path === '') throw new Error('deleteFolder: path kosong tidak boleh dikirim');
  return del(`/api/projects/${encodeURIComponent(projectId)}/knowledge/folders`, { query: { path }, schema: DeleteFolderResultSchema });
}
