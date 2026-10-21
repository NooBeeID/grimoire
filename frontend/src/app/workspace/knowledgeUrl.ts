// State tab Knowledge hidup di query URL (path memuat "/", jadi bukan segmen route).
export interface KnowledgeUrlState {
  path: string | null;
  dir: string | null;
  mode: 'view' | 'edit' | 'new';
  q: string;
  scope: string;
}

export function parseKnowledgeQuery(q: URLSearchParams): KnowledgeUrlState {
  const isNew = q.get('new') === '1';
  const path = isNew ? null : q.get('path') || null;
  const mode: KnowledgeUrlState['mode'] = isNew ? 'new' : q.get('mode') === 'edit' && path ? 'edit' : 'view';
  return { path, dir: q.get('dir') || null, mode, q: q.get('q') ?? '', scope: q.get('in') ?? '' };
}

export function buildKnowledgeHref(projectId: string, s: Partial<KnowledgeUrlState>): string {
  const p = new URLSearchParams();
  if (s.mode === 'new') p.set('new', '1');
  else if (s.path) p.set('path', s.path);
  if (s.dir) p.set('dir', s.dir);
  if (s.mode === 'edit' && s.path) p.set('mode', 'edit');
  if (s.q) p.set('q', s.q);
  if (s.scope) p.set('in', s.scope);
  const qs = p.toString();
  return `/projects/${encodeURIComponent(projectId)}/knowledge${qs ? `?${qs}` : ''}`;
}
