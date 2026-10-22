import { z } from 'zod';

export const KINDS = ['knowledge', 'api_spec'] as const;
export type Kind = (typeof KINDS)[number];
export const SETTINGS_PATH = '/settings#token-git';

export const RUN_STATUSES = ['success', 'partial', 'failed'] as const;
export const ITEM_STATUSES = ['created', 'updated', 'unchanged', 'skipped', 'rejected'] as const;
export type RunStatus = (typeof RUN_STATUSES)[number];
export type ItemStatus = (typeof ITEM_STATUSES)[number];

export const GitSyncIssueSchema = z.object({
  code: z.string(),
  message: z.string(),
  pointer: z.string(),
  line: z.number().int(),
});
export const GitSyncReportItemSchema = z.object({
  repo_path: z.string(),
  target: z.string(),
  status: z.enum(ITEM_STATUSES),
  reason_code: z.string(),
  reason: z.string(),
  issues: z.array(GitSyncIssueSchema),
  openapi_version: z.string().optional(),
  title: z.string().optional(),
  endpoint_count: z.number().int().optional(),
});
export const GitSyncSummarySchema = z.object({
  created: z.number().int(),
  updated: z.number().int(),
  unchanged: z.number().int(),
  skipped: z.number().int(),
  rejected: z.number().int(),
});
export const GitSyncReportSchema = z.object({
  status: z.enum(RUN_STATUSES),
  summary: GitSyncSummarySchema,
  items: z.array(GitSyncReportItemSchema),
  truncated: z.boolean(),
  error_code: z.string(),
  error_message: z.string(),
});
export const GitSyncConfigSchema = z.object({
  provider: z.enum(['github', 'gitlab']),
  host: z.string(),
  repo_url: z.string(),
  repo_path: z.string(),
  branch: z.string(),
  folder: z.string(),
  target_prefix: z.string(),
  last_status: z.enum(RUN_STATUSES).nullable(),
  last_synced_at: z.string().datetime({ offset: true }).nullable(),
  last_synced_by: z.object({ id: z.string(), label: z.string() }).nullable(),
  last_result: GitSyncReportSchema.nullable(),
});

export type GitSyncIssue = z.infer<typeof GitSyncIssueSchema>;
export type GitSyncReportItem = z.infer<typeof GitSyncReportItemSchema>;
export type GitSyncSummary = z.infer<typeof GitSyncSummarySchema>;
export type GitSyncReport = z.infer<typeof GitSyncReportSchema>;
export type GitSyncConfig = z.infer<typeof GitSyncConfigSchema>;
