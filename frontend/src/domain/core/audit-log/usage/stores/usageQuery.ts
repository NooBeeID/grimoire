import { DEFAULT_USAGE_RANGE, USAGE_RANGES, type UsageFilter, type UsageRange } from '../types';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// Kunci URL sama dengan Dashboard: range, project (uuid).
export function usageFilterFromQuery(q: URLSearchParams): UsageFilter {
  const r = q.get('range');
  const range = (USAGE_RANGES as readonly string[]).includes(r ?? '') ? (r as UsageRange) : DEFAULT_USAGE_RANGE;
  const p = q.get('project');
  return { range, projectId: p && UUID.test(p) ? p : null };
}

export const usageFilterKey = (f: UsageFilter) => `${f.range}|${f.projectId ?? ''}`;
