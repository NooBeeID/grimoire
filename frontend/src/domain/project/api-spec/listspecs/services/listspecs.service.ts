import { get } from '../../../../../shared/services/http';
import { ApiSpecListSchema, type ApiSpecList } from '../types';

export function listSpecs(projectId: string, signal?: AbortSignal): Promise<ApiSpecList> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/api-specs`, { schema: ApiSpecListSchema, signal });
}
