import { get } from '../../../../../shared/services/http';
import { truncateRunes } from '../../../../../shared/utils/text';
import { MAX_QUERY, PAGE_SIZE, ProjectListSchema, type ProjectList, type ProjectListQuery } from '../types';

export function parseListQuery(qs: URLSearchParams): ProjectListQuery {
  const q = truncateRunes((qs.get('q') ?? '').trim(), MAX_QUERY);
  const raw = qs.get('page') ?? '';
  const n = /^\d+$/.test(raw) ? Number(raw) : 1;
  return { q, page: Number.isSafeInteger(n) && n >= 1 ? n : 1 };
}

export function toSearchParams(q: ProjectListQuery): URLSearchParams {
  const p = new URLSearchParams();
  if (q.q) p.set('q', q.q);
  if (q.page > 1) p.set('page', String(q.page));
  return p;
}

export function listProjects(q: ProjectListQuery, signal?: AbortSignal): Promise<ProjectList> {
  return get('/api/projects', {
    query: { q: q.q || undefined, limit: PAGE_SIZE, offset: (q.page - 1) * PAGE_SIZE },
    schema: ProjectListSchema,
    signal,
  });
}
