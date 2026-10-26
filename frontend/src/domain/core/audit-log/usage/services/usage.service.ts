import { get } from '../../../../../shared/services/http';
import { AgentUsageSchema, type AgentUsage, type UsageFilter, type UsageSection } from '../types';

export function getAgentUsage(f: UsageFilter, section: UsageSection, signal?: AbortSignal): Promise<AgentUsage> {
  return get('/api/audit-log/usage', {
    query: { range: f.range, project_id: f.projectId ?? undefined, section },
    schema: AgentUsageSchema,
    signal,
  });
}
