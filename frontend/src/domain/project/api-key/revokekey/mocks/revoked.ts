// Data mock sesuai skema RevokeApiKeyResponse (api/modules/project/api-key.yml).
export const mockRevoked = (over: Record<string, unknown> = {}) => ({
  key: {
    id: '0192f5a0-2222-7aaa-8bbb-000000000003',
    name: 'ci-pipeline',
    prefix: 'a1b2c3d4',
    status: 'revoked',
    created_by: '0192f5a0-aaaa-7aaa-8bbb-00000000000a',
    created_by_label: 'ana@tim.id',
    created_at: '2026-10-23T10:00:00Z',
    last_used_at: '2026-10-23T11:42:00Z',
    revoked_at: '2026-10-23T12:00:00Z',
    revoked_by: '0192f5a0-bbbb-7aaa-8bbb-00000000000b',
    revoked_by_label: 'budi@tim.id',
    ...over,
  },
});
