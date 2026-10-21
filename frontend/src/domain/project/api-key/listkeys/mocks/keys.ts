// Data mock sesuai skema ApiKeyList (api/modules/project/api-key.yml).
import type { ApiKey } from '../types';

export const KEY_ID = (n: number) => `0192f5a0-2222-7aaa-8bbb-${String(n).padStart(12, '0')}`;

export const mockKey = (n: number, over: Partial<ApiKey> = {}): ApiKey => ({
  id: KEY_ID(n),
  name: 'ci-pipeline',
  prefix: 'a1b2c3d4',
  status: 'active',
  created_by: '0192f5a0-aaaa-7aaa-8bbb-00000000000a',
  created_by_label: 'ana@tim.id',
  created_at: '2026-10-23T10:00:00Z',
  last_used_at: '2026-10-23T11:42:00Z',
  revoked_at: null,
  revoked_by: null,
  revoked_by_label: null,
  ...over,
});

export const mockRevokedKey = (n: number, over: Partial<ApiKey> = {}): ApiKey =>
  mockKey(n, {
    name: 'claude-code-budi',
    prefix: 'k3x9q2ma',
    status: 'revoked',
    created_by_label: 'budi@tim.id',
    created_at: '2026-10-23T09:00:00Z',
    last_used_at: null,
    revoked_at: '2026-10-23T12:00:00Z',
    revoked_by: '0192f5a0-aaaa-7aaa-8bbb-00000000000a',
    revoked_by_label: 'ana@tim.id',
    ...over,
  });

export const mockKeyList = (...items: ApiKey[]) => ({ items });
