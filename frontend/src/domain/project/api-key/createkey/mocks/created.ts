// Data mock sesuai skema CreateApiKeyResponse (api/modules/project/api-key.yml).
export const MOCK_RAW_KEY = 'gr_k3x9q2ma_Q2xhdWRlLWNvZGUtYnVkaS1zZWNyZXQtZXhhbXBsZS0';

export const mockCreated = (over: Record<string, unknown> = {}) => ({
  key: {
    id: '0192f5a0-2222-7aaa-8bbb-000000000001',
    name: 'claude-code-budi',
    prefix: 'k3x9q2ma',
    status: 'active',
    created_by: '0192f5a0-bbbb-7aaa-8bbb-00000000000b',
    created_by_label: 'budi@tim.id',
    created_at: '2026-10-23T09:00:00Z',
    last_used_at: null,
    revoked_at: null,
    revoked_by: null,
    revoked_by_label: null,
  },
  raw_key: MOCK_RAW_KEY,
  mcp_url: 'https://grimoire.tim.id/mcp' as string | null,
  ...over,
});
