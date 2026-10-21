import { afterEach, describe, expect, it, vi } from 'vitest';
import { rootLevel } from '../mocks/tree';
import { getTreeLevel } from './listtree.service';

afterEach(() => vi.unstubAllGlobals());
const ok = (b: unknown) => vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(b), { status: 200 }));

describe('getTreeLevel', () => {
  it('encodes project id and prefix', async () => {
    const f = ok({ ...rootLevel, prefix: 'docs/auth' });
    vi.stubGlobal('fetch', f);
    await getTreeLevel('p/1', 'docs/auth');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p%2F1/knowledge/tree?prefix=docs%2Fauth');
  });
  it('omits prefix at root', async () => {
    const f = ok(rootLevel);
    vi.stubGlobal('fetch', f);
    await getTreeLevel('p1', '');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/knowledge/tree');
  });
  it('malformed response -> INVALID_RESPONSE', async () => {
    vi.stubGlobal('fetch', ok({ prefix: '' }));
    await expect(getTreeLevel('p1', '')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
