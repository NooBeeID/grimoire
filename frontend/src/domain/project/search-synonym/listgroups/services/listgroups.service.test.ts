import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockGroupList } from '../mocks/groups';
import { listGroups } from './listgroups.service';

afterEach(() => vi.unstubAllGlobals());
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });

describe('listGroups', () => {
  it('GETs the project synonyms URL and returns validated items', async () => {
    const f = vi.fn(async () => json(200, mockGroupList()));
    vi.stubGlobal('fetch', f);
    const r = await listGroups('p/1');
    expect(r.items).toHaveLength(3);
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p%2F1/search/synonyms');
    expect(init.method).toBe('GET');
  });
  it('rejects an invalid response with INVALID_RESPONSE', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { items: [{ id: 'x' }] })));
    await expect(listGroups('p1')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('maps 404 to ApiError status 404', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x' } })));
    await expect(listGroups('p1')).rejects.toMatchObject({ status: 404 });
  });
});
