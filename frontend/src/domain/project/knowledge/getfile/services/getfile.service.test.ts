import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockFile } from '../../base';
import { getFile } from './getfile.service';

afterEach(() => vi.unstubAllGlobals());

describe('getFile', () => {
  it('encodes unicode and spaces in path', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(mockFile('catatan rapat/é.md')), { status: 200 }));
    vi.stubGlobal('fetch', f);
    const r = await getFile('p1', 'catatan rapat/é.md');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/knowledge/files?path=catatan+rapat%2F%C3%A9.md');
    expect(r.version).toHaveLength(64);
  });
  it('rejects a bad version with INVALID_RESPONSE', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(mockFile('a.md', { version: 'zz' })), { status: 200 })));
    await expect(getFile('p1', 'a.md')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
