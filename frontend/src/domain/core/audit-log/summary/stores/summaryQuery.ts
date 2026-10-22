import { DEFAULT_SUMMARY_RANGE, SUMMARY_RANGES, type SummaryFilter, type SummaryRange } from '../types';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function summaryFilterFromQuery(q: URLSearchParams): SummaryFilter {
  const r = q.get('range');
  const range = (SUMMARY_RANGES as readonly string[]).includes(r ?? '') ? (r as SummaryRange) : DEFAULT_SUMMARY_RANGE;
  const p = q.get('project');
  return { range, projectId: p && UUID.test(p) ? p : null };
}

export function summaryFilterToQuery(f: SummaryFilter): URLSearchParams {
  const q = new URLSearchParams();
  q.set('range', f.range);
  if (f.projectId) q.set('project', f.projectId);
  return q;
}

export const summaryFilterKey = (f: SummaryFilter) => `${f.range}|${f.projectId ?? ''}`;
