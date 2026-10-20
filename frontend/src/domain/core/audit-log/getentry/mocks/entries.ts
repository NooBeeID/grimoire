// Data mock sesuai skema AuditEntryDetail (api/modules/core/audit-log.yml).
// Dipakai tes sampai backend T02 tersedia.
export const mockDetail = {
  id: '0192f0c4-7b00-7d11-8a00-1a2b3c4d5e6f',
  occurred_at: '2026-10-19T08:14:22.123456Z',
  actor_type: 'api_key' as const,
  actor_id: '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c',
  actor_label: 'claude-desktop',
  project_id: '0192f0c4-0000-7000-8000-000000000001',
  project_label: 'Grimoire Docs',
  action: 'mcp.read_file',
  target_type: 'file',
  target: 'docs/login.md',
  status: 'ok' as const,
  args: { path: 'docs/login.md', api_key: '[REDACTED]' },
  result: { bytes: 2048 },
  ip: '203.0.113.5',
  user_agent: 'claude-desktop/1.0',
  trace_id: 'trace-abc123',
};
