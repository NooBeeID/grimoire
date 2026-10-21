import { post } from '../../../../../shared/services/http';
import { RevokeKeyResponseSchema, type RevokedKey } from '../types';

export async function revokeKey(projectId: string, keyId: string): Promise<RevokedKey> {
  const r = await post(
    `/api/projects/${encodeURIComponent(projectId)}/api-keys/${encodeURIComponent(keyId)}/revoke`,
    undefined,
    { schema: RevokeKeyResponseSchema },
  );
  return r.key;
}
