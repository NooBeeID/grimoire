import type { ApiSpecEndpoint } from '../types';

// Semua token (dipisah spasi) harus cocok pada gabungan method, path, operationId, summary, dan tag.
export function filterEndpoints(endpoints: ApiSpecEndpoint[], q: string): ApiSpecEndpoint[] {
  const tokens = q.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return endpoints;
  return endpoints.filter((e) => {
    const hay = `${e.method} ${e.path} ${e.operation_id ?? ''} ${e.summary} ${e.tags.join(' ')}`.toLowerCase();
    return tokens.every((t) => hay.includes(t));
  });
}
