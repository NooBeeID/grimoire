import { post } from '../../../../../shared/services/http';
import { CreateKeyResponseSchema, type CreateKeyResponse } from '../types';

// Tanpa AbortSignal: setelah terkirim, hasilnya satu-satunya kesempatan melihat key.
export function createKey(projectId: string, name: string): Promise<CreateKeyResponse> {
  return post(`/api/projects/${encodeURIComponent(projectId)}/api-keys`, { name }, { schema: CreateKeyResponseSchema });
}
