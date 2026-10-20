import { z } from 'zod';
import type { ApiError } from '../../../../shared/services/http';

// Salinan lokal (rule 12.4: tidak import dari feature lain).
export const ACTOR_TYPES = ['user', 'api_key', 'anonymous', 'system'] as const;
export const ENTRY_STATUSES = ['ok', 'error', 'denied'] as const;

// Schema respons GET /api/audit-log/{id}; dipakai service untuk validasi.
export const AuditEntryDetailSchema = z.object({
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
  status: z.enum(ENTRY_STATUSES),
  args: z.record(z.string(), z.unknown()),
  result: z.record(z.string(), z.unknown()),
  ip: z.string().nullable(),
  user_agent: z.string().nullable(),
  trace_id: z.string(),
});

export type AuditEntryDetail = z.infer<typeof AuditEntryDetailSchema>;
export type DetailError = 'notFound' | 'forbidden' | 'load';

export type DetailState =
  | { status: 'idle' }
  | { status: 'loading'; id: string }
  | { status: 'ready'; id: string; entry: AuditEntryDetail }
  | { status: 'error'; id: string; error: DetailError; apiError: ApiError };
