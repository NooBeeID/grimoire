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

export const ANA_ID = '0192f5a0-aaaa-7aaa-8bbb-00000000000a';
export const BUDI_ID = '0192f5a0-bbbb-7aaa-8bbb-00000000000b';
export const ADMIN_ID = '0192f5a0-cccc-7aaa-8bbb-00000000000c';

// Project "Payment Gateway Docs": tampilan Super Admin (lima key, termasuk legacy tanpa pemilik).
export const mockTeamKeys = (): ApiKey[] => [
  mockKey(1, { name: 'claude-code-ana', prefix: 'a1b2c3d4', created_by: ANA_ID, created_by_label: 'ana@tim.id' }),
  mockKey(2, {
    name: 'ci-pipeline',
    prefix: 'm7p2x9qa',
    created_by: ANA_ID,
    created_by_label: 'ana@tim.id',
    created_at: '2026-10-22T08:15:00Z',
    last_used_at: null,
  }),
  mockRevokedKey(3, {
    name: 'claude-code-budi',
    prefix: 'k3x9q2ma',
    created_by: BUDI_ID,
    created_by_label: 'budi@tim.id',
    created_at: '2026-10-21T09:00:00Z',
    last_used_at: '2026-10-21T16:05:00Z',
    revoked_by: ADMIN_ID,
    revoked_by_label: 'admin@tim.id',
  }),
  mockKey(4, {
    name: 'cursor-budi',
    prefix: 'p4q8r2zt',
    created_by: BUDI_ID,
    created_by_label: 'budi@tim.id',
    created_at: '2026-10-23T13:00:00Z',
    last_used_at: null,
  }),
  mockKey(5, {
    name: 'legacy-key',
    prefix: 'z9y8x7w6',
    created_by: null,
    created_by_label: null,
    created_at: '2026-10-20T08:00:00Z',
    last_used_at: null,
  }),
];
