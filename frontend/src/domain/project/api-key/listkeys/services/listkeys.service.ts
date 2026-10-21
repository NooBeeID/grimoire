import { get } from '../../../../../shared/services/http';
import { ApiKeyListSchema, type ApiKeyList } from '../types';

export function listKeys(projectId: string, signal?: AbortSignal): Promise<ApiKeyList> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/api-keys`, { schema: ApiKeyListSchema, signal });
}
