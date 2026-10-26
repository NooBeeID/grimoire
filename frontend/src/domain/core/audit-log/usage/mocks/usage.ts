// Data mock sesuai skema GET /api/audit-log/usage (plan T19); dipakai tes sampai backend siap.
import type { AgentUsage } from '../types';

export const PROJECT_ID = '0192f5a0-1111-7aaa-8bbb-000000000001';

const base = { range: '7d', from: '2026-10-19T10:30:15Z', to: '2026-10-26T10:30:15Z' } as const;

export const mockTopFiles = [
  { path: 'docs/payments/refund-flow.md', reads: 48, keys: 3, last_read_at: '2026-10-26T09:12:00Z' },
  { path: 'docs/payments/webhook-signature.md', reads: 31, keys: 2, last_read_at: '2026-10-26T08:40:00Z' },
  { path: 'runbooks/ledger-reconcile.md', reads: 12, keys: 1, last_read_at: '2026-10-25T17:05:00Z' },
];
export const mockUnreadFiles = [
  { path: 'docs/payments/legacy-settlement.md', updated_at: '2026-08-02T04:00:00Z' },
  { path: 'runbooks/partner-onboarding.md', updated_at: '2026-09-14T11:20:00Z' },
];
export const mockZeroQueries = [
  { query: 'idempotency key retry', count: 7, keys: 2, last_at: '2026-10-26T07:55:00Z' },
  { query: 'settlement cutoff time', count: 4, keys: 1, last_at: '2026-10-24T13:10:00Z' },
];
export const mockKeys = [
  {
    key_id: '0192f5a0-2222-7aaa-8bbb-000000000001',
    label: 'ci-review-agent',
    calls: 412,
    errors: 9,
    searches: 120,
    reads: 260,
    last_used_at: '2026-10-26T10:02:00Z',
    top_tool: 'read_file',
  },
  {
    key_id: '0192f5a0-2222-7aaa-8bbb-000000000002',
    label: 'support-triage',
    calls: 96,
    errors: 0,
    searches: 70,
    reads: 20,
    last_used_at: '2026-10-25T16:30:00Z',
    top_tool: 'search_knowledge',
  },
];

export const mockUsageAll: AgentUsage = {
  ...base,
  project_id: PROJECT_ID,
  top_files: { available: true, items: mockTopFiles },
  unread_files: { available: true, truncated: false, items: mockUnreadFiles },
  zero_queries: { available: true, items: mockZeroQueries },
  per_key: { available: true, items: mockKeys },
};

export const mockUsageNoProject: AgentUsage = {
  ...mockUsageAll,
  project_id: null,
  top_files: { available: false, error_code: 'PROJECT_REQUIRED', items: [] },
  unread_files: { available: false, error_code: 'PROJECT_REQUIRED', truncated: false, items: [] },
};

export const mockUsageEmpty: AgentUsage = {
  ...mockUsageAll,
  top_files: { available: true, items: [] },
  unread_files: { available: true, truncated: false, items: [] },
  zero_queries: { available: true, items: [] },
  per_key: { available: true, items: [] },
};

// Respons satu bagian (?section=): bagian lain null.
export function mockUsageSection(
  full: AgentUsage,
  section: 'top_files' | 'unread_files' | 'zero_queries' | 'keys',
): AgentUsage {
  const only = { ...full, top_files: null, unread_files: null, zero_queries: null, per_key: null };
  const k = section === 'keys' ? 'per_key' : section;
  return { ...only, [k]: full[k] } as AgentUsage;
}
