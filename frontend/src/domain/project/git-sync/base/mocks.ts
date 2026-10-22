// Data mock sesuai skema api/modules/project/git-sync.yml.
import type { GitSyncConfig, GitSyncReport, GitSyncReportItem, Kind } from './types';

const item = (over: Partial<GitSyncReportItem>): GitSyncReportItem => ({
  repo_path: '',
  target: '',
  status: 'created',
  reason_code: '',
  reason: '',
  issues: [],
  ...over,
});

export const mockKnowledgeReport = (over: Partial<GitSyncReport> = {}): GitSyncReport => ({
  status: 'partial',
  summary: { created: 2, updated: 0, unchanged: 0, skipped: 1, rejected: 1 },
  items: [
    item({ repo_path: 'docs/api/rate-limit.md', target: 'api/rate-limit.md' }),
    item({ repo_path: 'docs/auth/login-flow.md', target: 'auth/login-flow.md' }),
    item({ repo_path: 'docs/img/diagram.png', status: 'skipped', reason_code: 'unsupported_type' }),
    item({ repo_path: 'docs/legacy/../x.md', status: 'rejected', reason_code: 'path_traversal' }),
  ],
  truncated: false,
  error_code: '',
  error_message: '',
  ...over,
});

export const mockApiSpecReport = (over: Partial<GitSyncReport> = {}): GitSyncReport => ({
  status: 'partial',
  summary: { created: 1, updated: 0, unchanged: 0, skipped: 0, rejected: 1 },
  items: [
    item({ repo_path: 'api/openapi/billing.yaml', target: 'billing', openapi_version: '3.0.3', title: 'Billing API', endpoint_count: 14 }),
    item({
      repo_path: 'api/openapi/payments.json',
      target: 'payments',
      status: 'rejected',
      reason_code: 'INVALID_SPEC',
      reason: '$ref eksternal tidak didukung',
      issues: [{ code: 'EXTERNAL_REF', message: '$ref eksternal tidak didukung', pointer: '#/paths/~1charges/get', line: 0 }],
    }),
  ],
  truncated: false,
  error_code: '',
  error_message: '',
  ...over,
});

export const mockConfig = (over: Partial<GitSyncConfig> = {}): GitSyncConfig => ({
  provider: 'github',
  host: 'github.com',
  repo_url: 'https://github.com/noobee/grimoire-docs/tree/main/docs',
  repo_path: 'noobee/grimoire-docs',
  branch: 'main',
  folder: 'docs',
  target_prefix: '',
  last_status: 'partial',
  last_synced_at: '2026-10-22T14:05:00Z',
  last_synced_by: { id: '0192f5a0-aaaa-7aaa-8bbb-00000000000a', label: 'Rina' },
  last_result: mockKnowledgeReport(),
  ...over,
});

// Respons GET /git-sync (api/modules/project/git-sync.yml 0.2.0).
export const mockGetConfigResponse = (
  over: Partial<{ kind: Kind; feature_enabled: boolean; has_credential: boolean; credential_unreadable: boolean; in_progress: boolean; config: GitSyncConfig | null }> = {},
) => ({
  kind: 'knowledge' as Kind,
  feature_enabled: true,
  has_credential: true,
  credential_unreadable: false,
  in_progress: false,
  config: mockConfig() as GitSyncConfig | null,
  ...over,
});

// Event SSE mentah (nama, data) untuk mock stream.
export const mockSyncStream = (total = 3): [string, unknown][] => [
  ['start', { total }],
  ...Array.from({ length: total }, (_, i): [string, unknown] => ['progress', { done: i + 1, total, path: `docs/auth/file-${i + 1}.md` }]),
  ['saving', {}],
  ['done', { report: mockKnowledgeReport(), config: mockConfig({ last_result: null }) }],
];
