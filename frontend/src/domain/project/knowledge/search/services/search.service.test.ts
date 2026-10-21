import { afterEach, describe, expect, it, vi } from 'vitest';
import { searchKnowledge } from './search.service';

afterEach(() => vi.unstubAllGlobals());
const stub = () => {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify({ items: [], total: 0 }), { status: 200 }));
  vi.stubGlobal('fetch', f);
  return f;
};

describe('searchKnowledge', () => {
  it('sends q, limit and omits empty prefix', async () => {
    const f = stub();
    await searchKnowledge('p1', { q: 'login', prefix: '', limit: 20 });
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/knowledge/search?q=login&limit=20');
  });
  it('includes prefix and truncates queries over 200 characters', async () => {
    const f = stub();
    await searchKnowledge('p1', { q: 'x'.repeat(300), prefix: 'docs', limit: 50 });
    const u = new URL(f.mock.calls[0]![0] as string, 'http://x');
    expect(u.searchParams.get('q')).toHaveLength(200);
    expect(u.searchParams.get('prefix')).toBe('docs');
  });
});
