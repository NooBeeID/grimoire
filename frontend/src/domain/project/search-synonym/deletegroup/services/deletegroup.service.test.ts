import { afterEach, describe, expect, it, vi } from 'vitest';
import { deleteGroup } from './deletegroup.service';

afterEach(() => vi.unstubAllGlobals());

describe('deleteGroup', () => {
  it('DELETEs the group URL and resolves on 204', async () => {
    const f = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', f);
    await expect(deleteGroup('p1', 'g1')).resolves.toBeUndefined();
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p1/search/synonyms/g1');
    expect(init.method).toBe('DELETE');
  });
  it('404 rejects with status 404', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x' } }), { status: 404 })));
    await expect(deleteGroup('p1', 'g1')).rejects.toMatchObject({ status: 404 });
  });
});
