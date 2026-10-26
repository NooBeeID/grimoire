import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SynonymGroup } from '../../listgroups';
import { createGroup, updateGroup } from './savegroup.service';

const mockGroup = (n: number, over: Partial<SynonymGroup> = {}): SynonymGroup => ({
  id: `0192f5a0-3333-7aaa-8bbb-${String(n).padStart(12, '0')}`,
  terms: ['login', 'autentikasi', 'masuk'],
  created_at: '2026-10-23T10:00:00Z',
  updated_at: '2026-10-23T10:00:00Z',
  created_by_label: 'ana@tim.id',
  updated_by_label: 'ana@tim.id',
  ...over,
});


afterEach(() => vi.unstubAllGlobals());
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });

describe('savegroup service', () => {
  it('createGroup POSTs terms with CSRF header', async () => {
    const f = vi.fn(async () => json(201, mockGroup(1)));
    vi.stubGlobal('fetch', f);
    const g = await createGroup('p1', ['login', 'autentikasi']);
    expect(g.id).toBe(mockGroup(1).id);
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p1/search/synonyms');
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toEqual({ terms: ['login', 'autentikasi'] });
    expect(new Headers(init.headers).get('X-Requested-With')).toBe('grimoire');
  });
  it('updateGroup PUTs to the group URL', async () => {
    const f = vi.fn(async () => json(200, mockGroup(1)));
    vi.stubGlobal('fetch', f);
    await updateGroup('p1', mockGroup(1).id, ['a', 'b']);
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe(`/api/projects/p1/search/synonyms/${mockGroup(1).id}`);
    expect(init.method).toBe('PUT');
  });
  it('invalid response -> INVALID_RESPONSE', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(201, { id: 'x' })));
    await expect(createGroup('p1', ['a', 'b'])).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('409 term_in_use keeps details', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => json(409, { error: { code: 'CONFLICT', message: 'x', details: { reason: 'term_in_use', terms: ['login'] } } })),
    );
    await expect(createGroup('p1', ['login', 'masuk'])).rejects.toMatchObject({ status: 409, details: { reason: 'term_in_use' } });
  });
});
