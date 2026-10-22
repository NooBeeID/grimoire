// Data mock sesuai skema AuditSummary (api/modules/core/audit-log.yml).
import type { UsageSummary } from '../types';

export const PROJECT_IDS = {
  payments: '0192f5a0-1111-7aaa-8bbb-000000000001',
  onboarding: '0192f5a0-1111-7aaa-8bbb-000000000002',
  ledger: '0192f5a0-1111-7aaa-8bbb-000000000003',
  partner: '0192f5a0-1111-7aaa-8bbb-000000000004',
  docs: '0192f5a0-1111-7aaa-8bbb-000000000005',
} as const;

const tools = [
  { tool: 'read_file', calls: 600, error: 10, p50_ms: 8, p95_ms: 40 },
  { tool: 'search_knowledge', calls: 402, error: 16, p50_ms: 35, p95_ms: 180 },
  { tool: 'list_files', calls: 150, error: 0, p50_ms: 5, p95_ms: 22 },
  { tool: 'get_api_endpoint', calls: 82, error: 5, p50_ms: null, p95_ms: null },
];
const toolMetrics = tools.map((x) => ({
  tool: x.tool,
  calls: x.calls,
  ok: x.calls - x.error,
  error: x.error,
  error_rate: x.error / x.calls,
  p50_ms: x.p50_ms,
  p95_ms: x.p95_ms,
}));
const totalCalls = tools.reduce((s, x) => s + x.calls, 0);
const totalErrors = tools.reduce((s, x) => s + x.error, 0);
const trafficCalls = [90, 0, 210, 188, 175, 240, 221, 110];
const trafficErrors = [2, 0, 5, 4, 3, 7, 6, 4];

export const mockSummary7d: UsageSummary = {
  range: '7d',
  from: '2026-10-15T10:30:15Z',
  to: '2026-10-22T10:30:15Z',
  bucket: 'day',
  project_id: null,
  total_calls: totalCalls,
  total_errors: totalErrors,
  error_rate: totalErrors / totalCalls,
  top_projects: [
    { project_id: PROJECT_IDS.payments, project_label: 'Payments', calls: 420 },
    { project_id: PROJECT_IDS.onboarding, project_label: 'Onboarding Mobile', calls: 311 },
    { project_id: PROJECT_IDS.ledger, project_label: 'Ledger Core', calls: 268 },
    { project_id: PROJECT_IDS.partner, project_label: 'Partner Portal', calls: 150 },
    { project_id: PROJECT_IDS.docs, project_label: 'Grimoire Docs', calls: 85 },
  ],
  tool_usage: tools.map((x) => ({ tool: x.tool, calls: x.calls })),
  tool_metrics: toolMetrics,
  traffic: trafficCalls.map((calls, i) => ({
    at: new Date(Date.UTC(2026, 9, 15 + i)).toISOString().replace('.000Z', 'Z'),
    calls,
    errors: trafficErrors[i] ?? 0,
  })),
  search_quality: null,
};

const hourly = Array.from({ length: 25 }, (_, i) => {
  const at = new Date(Date.UTC(2026, 9, 21, 10 + i, 0, 0)).toISOString().replace('.000Z', 'Z');
  const calls = i >= 6 && i <= 20 ? 3 + ((i * 7) % 5) : 0;
  return { at, calls, errors: i === 12 ? 1 : 0 };
});

export const mockSummary24hProject: UsageSummary = {
  range: '24h',
  from: '2026-10-21T10:30:15Z',
  to: '2026-10-22T10:30:15Z',
  bucket: 'hour',
  project_id: PROJECT_IDS.payments,
  total_calls: 65,
  total_errors: 1,
  error_rate: 1 / 65,
  top_projects: [{ project_id: PROJECT_IDS.payments, project_label: 'Payments', calls: 65 }],
  tool_usage: [
    { tool: 'search_knowledge', calls: 60 },
    { tool: 'read_file', calls: 5 },
  ],
  tool_metrics: [
    { tool: 'search_knowledge', calls: 60, ok: 59, error: 1, error_rate: 1 / 60, p50_ms: 31, p95_ms: 150 },
    { tool: 'read_file', calls: 5, ok: 5, error: 0, error_rate: 0, p50_ms: 7, p95_ms: 12 },
  ],
  traffic: hourly,
  search_quality: {
    available: true,
    searches: 59,
    window_seconds: 120,
    zero_result: { count: 4, denominator: 59, rate: 4 / 59 },
    follow_up: { count: 31, denominator: 55, rate: 31 / 55 },
    reformulation: { count: 7, denominator: 59, rate: 7 / 59 },
    rank_opened: [
      ...[18, 6, 3, 1, 1, 0, 0, 0, 0, 0].map((count, i) => ({ label: String(i + 1), count })),
      { label: 'not_in_results', count: 2 },
    ],
    legacy_searches: 0,
  },
};

export const mockSummaryQualityUnavailable: UsageSummary = {
  ...mockSummary24hProject,
  search_quality: { available: false, error_code: 'UNAVAILABLE' },
};

export const mockSummaryEmpty: UsageSummary = {
  range: '7d',
  from: '2026-10-15T10:30:15Z',
  to: '2026-10-22T10:30:15Z',
  bucket: 'day',
  project_id: PROJECT_IDS.ledger,
  total_calls: 0,
  total_errors: 0,
  error_rate: null,
  top_projects: [],
  tool_usage: [],
  tool_metrics: [],
  traffic: trafficCalls.map((_, i) => ({
    at: new Date(Date.UTC(2026, 9, 15 + i)).toISOString().replace('.000Z', 'Z'),
    calls: 0,
    errors: 0,
  })),
  search_quality: {
    available: true,
    searches: 0,
    window_seconds: 120,
    zero_result: { count: 0, denominator: 0, rate: 0 },
    follow_up: { count: 0, denominator: 0, rate: 0 },
    reformulation: { count: 0, denominator: 0, rate: 0 },
    rank_opened: [
      ...Array.from({ length: 10 }, (_, i) => ({ label: String(i + 1), count: 0 })),
      { label: 'not_in_results', count: 0 },
    ],
    legacy_searches: 0,
  },
};

export const mockProjectOptions = {
  items: [
    { id: PROJECT_IDS.payments, name: 'Payments' },
    { id: PROJECT_IDS.onboarding, name: 'Onboarding Mobile' },
    { id: PROJECT_IDS.ledger, name: 'Ledger Core' },
    { id: PROJECT_IDS.partner, name: 'Partner Portal' },
    { id: PROJECT_IDS.docs, name: 'Grimoire Docs' },
  ],
  total: 5,
};
