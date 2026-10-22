import { get } from '../../../../../shared/services/http';
import { ProjectOptionListSchema, type ProjectOption } from '../types';

export const PROJECT_OPTIONS_LIMIT = 100;

export async function listProjectOptions(signal?: AbortSignal): Promise<{ options: ProjectOption[]; total: number }> {
  const res = await get('/api/projects', { query: { limit: PROJECT_OPTIONS_LIMIT }, schema: ProjectOptionListSchema, signal });
  const options = [...res.items].sort((a, b) => a.name.localeCompare(b.name));
  return { options, total: res.total };
}
