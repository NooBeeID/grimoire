import { get } from '../../../../../shared/services/http';
import { ApiSpecDetailSchema, type ApiSpecDetail } from '../types';

export function getSpec(projectId: string, name: string, signal?: AbortSignal): Promise<ApiSpecDetail> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/api-specs/${encodeURIComponent(name)}`, {
    schema: ApiSpecDetailSchema,
    signal,
  });
}
