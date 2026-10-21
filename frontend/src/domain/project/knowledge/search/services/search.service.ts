import { get } from '../../../../../shared/services/http';
import { truncateRunes } from '../../../../../shared/utils/text';
import { MAX_QUERY_CHARS, SearchResultSchema, type SearchResult } from '../types';

export function searchKnowledge(
  projectId: string,
  p: { q: string; prefix: string; limit: number },
  signal?: AbortSignal,
): Promise<SearchResult> {
  return get(`/api/projects/${encodeURIComponent(projectId)}/knowledge/search`, {
    query: { q: truncateRunes(p.q, MAX_QUERY_CHARS), prefix: p.prefix || undefined, limit: p.limit },
    schema: SearchResultSchema,
    signal,
  });
}
