import { get } from '../../../../../shared/services/http';
import type { Kind } from '../../base';
import { GetConfigResponseSchema, type GetConfigResponse } from '../types';

export function getConfig(projectId: string, kind: Kind, signal?: AbortSignal): Promise<GetConfigResponse> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/git-sync`, { query: { kind }, schema: GetConfigResponseSchema, signal });
}
