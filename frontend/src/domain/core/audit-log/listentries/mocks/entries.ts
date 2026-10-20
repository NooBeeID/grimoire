// Data mock sesuai skema AuditList (api/modules/core/audit-log.yml).
export const mockSummary = (n: number, over: Record<string, unknown> = {}) => ({
  id: `0192f0c4-7b00-7d11-8a00-${String(n).padStart(12, '0')}`,
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
  ...over,
});

export const mockList = (count: number, nextCursor: string | null = null, start = 1) => ({
  items: Array.from({ length: count }, (_, i) => mockSummary(start + i)),
  next_cursor: nextCursor,
});
