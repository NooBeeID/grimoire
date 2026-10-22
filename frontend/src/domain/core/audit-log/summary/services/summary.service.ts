import { get } from '../../../../../shared/services/http';
import { UsageSummarySchema, type SummaryFilter, type UsageSummary } from '../types';

export function getUsageSummary(f: SummaryFilter, signal?: AbortSignal): Promise<UsageSummary> {
  return get('/api/audit-log/summary', {
    query: { range: f.range, project_id: f.projectId ?? undefined },
    schema: UsageSummarySchema,
    signal,
  });
}
