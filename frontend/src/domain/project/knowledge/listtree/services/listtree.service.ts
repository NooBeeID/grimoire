import { get } from '../../../../../shared/services/http';
import { TreeLevelSchema, type TreeLevel } from '../types';

export function getTreeLevel(projectId: string, prefix: string, signal?: AbortSignal): Promise<TreeLevel> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/knowledge/tree`, {
    query: { prefix: prefix || undefined },
    schema: TreeLevelSchema,
    signal,
  });
}
