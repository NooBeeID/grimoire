import type { z } from 'zod';
import { get } from '../../../../../shared/services/http';
import type { Kind } from '../../base';
import { GetConfigResponseSchema, type GetConfigResponse } from '../types';

export function getConfig(projectId: string, kind: Kind, signal?: AbortSignal): Promise<GetConfigResponse> {
  // in_progress memakai default(false): tipe input opsional, output wajib.
  return get<GetConfigResponse>(`/api/projects/${encodeURIComponent(projectId)}/git-sync`, { query: { kind }, schema: GetConfigResponseSchema as unknown as z.ZodType<GetConfigResponse>, signal });
}
