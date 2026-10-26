import { get } from '../../../../../shared/services/http';
import { SynonymGroupListSchema, type SynonymGroupList } from '../types';

export function listGroups(projectId: string, signal?: AbortSignal): Promise<SynonymGroupList> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/search/synonyms`, { schema: SynonymGroupListSchema, signal });
}
