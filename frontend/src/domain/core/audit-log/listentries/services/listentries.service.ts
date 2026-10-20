import { get } from '../../../../../shared/services/http';
import {
  AuditListSchema,
  AuditSummarySchema,
  PAGE_SIZE,
  RANGE_PRESETS,
  type AuditFilter,
  type AuditSummary,
} from '../types';

export { AuditListSchema, AuditSummarySchema };

export interface ResolvedRange {
  from?: string;
  to?: string;
}

function isoSeconds(d: Date): string {
  return d.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

export function resolveRange(f: AuditFilter, now: Date): ResolvedRange {
  if (f.range === 'custom') {
    return { ...(f.from ? { from: f.from } : {}), ...(f.to ? { to: f.to } : {}) };
  }
  const preset = RANGE_PRESETS.find((p) => p.key === f.range);
  if (preset) return { from: isoSeconds(new Date(now.getTime() - preset.ms)) };
  return {};
}

export function buildListQuery(
  f: AuditFilter,
  range: ResolvedRange,
  cursor: string | null,
  limit = PAGE_SIZE,
): string {
  const p = new URLSearchParams();
  if (f.actorType) {
    p.append('actor_type', f.actorType);
    if (f.actorId) p.append('actor_id', f.actorId);
  }
  if (f.actorLabel) p.append('actor_label', f.actorLabel);
  if (f.projectId) p.append('project_id', f.projectId);
  for (const a of f.actions) p.append('action', a);
  for (const s of f.statuses) p.append('status', s);
  if (range.from) p.append('from', range.from);
  if (range.to) p.append('to', range.to);
  if (f.target) p.append('target', f.target);
  if (cursor) p.append('cursor', cursor);
  p.append('limit', String(limit));
  const qs = p.toString();
  return qs ? `/api/audit-log?${qs}` : '/api/audit-log';
}

export async function listEntries(
  f: AuditFilter,
  range: ResolvedRange,
  cursor: string | null,
  signal?: AbortSignal,
): Promise<{ items: AuditSummary[]; nextCursor: string | null }> {
  const res = await get(buildListQuery(f, range, cursor), { schema: AuditListSchema, signal });
  return { items: res.items, nextCursor: res.next_cursor };
}
