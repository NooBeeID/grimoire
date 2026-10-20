import { z } from 'zod';

export const ACTOR_TYPES = ['user', 'api_key', 'anonymous', 'system'] as const;
export const AUDIT_STATUSES = ['ok', 'error', 'denied'] as const;
export type ActorType = (typeof ACTOR_TYPES)[number];
export type AuditStatus = (typeof AUDIT_STATUSES)[number];

// Sinkron dengan lib/contracts/core/audit/record/actions.go; provider tidak membatasi aksi.
// Aksi baru: tambahkan di sini dan di katalog README.
export const ACTION_GROUPS = [
  {
    key: 'mcp',
    actions: [
      'mcp.list_files',
      'mcp.read_file',
      'mcp.search_knowledge',
      'mcp.list_api_specs',
      'mcp.list_api_endpoints',
      'mcp.get_api_endpoint',
      'mcp.auth.rejected',
      'mcp.rate_limited',
    ],
  },
  {
    key: 'auth',
    actions: [
      'auth.login.success',
      'auth.login.failed',
      'auth.logout',
      'auth.password.changed',
      'auth.password.reset',
      'user.created',
      'user.updated',
      'user.deactivated',
      'user.activated',
    ],
  },
  {
    key: 'project',
    actions: [
      'project.created',
      'project.updated',
      'project.deleted',
      'project.member.added',
      'project.member.removed',
      'project.member.role_changed',
    ],
  },
  { key: 'apikey', actions: ['apikey.created', 'apikey.revoked'] },
  {
    key: 'content',
    actions: [
      'knowledge.file.created',
      'knowledge.file.updated',
      'knowledge.file.deleted',
      'knowledge.folder.deleted',
      'knowledge.upload.completed',
      'apispec.uploaded',
      'apispec.updated',
      'apispec.deleted',
    ],
  },
] as const;

export const AUDIT_ACTIONS: readonly string[] = ACTION_GROUPS.flatMap((g) => g.actions);

export const RANGE_PRESETS = [
  { key: '1h', ms: 3_600_000 },
  { key: '24h', ms: 86_400_000 },
  { key: '7d', ms: 604_800_000 },
  { key: '30d', ms: 2_592_000_000 },
  { key: '90d', ms: 7_776_000_000 },
] as const;
export type RangeKey = (typeof RANGE_PRESETS)[number]['key'] | 'all' | 'custom';

export interface AuditFilter {
  actorType: ActorType | '';
  actorId: string;
  actorLabel: string;
  projectId: string;
  actions: string[];
  statuses: AuditStatus[];
  range: RangeKey;
  from: string;
  to: string;
  target: string;
}

export const EMPTY_FILTER: AuditFilter = {
  actorType: '',
  actorId: '',
  actorLabel: '',
  projectId: '',
  actions: [],
  statuses: [],
  range: 'all',
  from: '',
  to: '',
  target: '',
};

export const MAX_ACTIONS = 20;
export const MAX_TEXT = 200;
export const PAGE_SIZE = 50;
export const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export const ACTION_RE = /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/;

export const AuditSummarySchema = z.object({
  id: z.string().uuid(),
  occurred_at: z.string().datetime({ offset: true }),
  actor_type: z.enum(ACTOR_TYPES),
  actor_id: z.string().nullable(),
  actor_label: z.string(),
  project_id: z.string().nullable(),
  project_label: z.string().nullable(),
  action: z.string(),
  target_type: z.string().nullable(),
  target: z.string().nullable(),
  status: z.enum(AUDIT_STATUSES),
});
export const AuditListSchema = z.object({
  items: z.array(AuditSummarySchema),
  next_cursor: z.string().nullable(),
});

export type AuditSummary = z.infer<typeof AuditSummarySchema>;
