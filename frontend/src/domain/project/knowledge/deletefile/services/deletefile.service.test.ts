import { afterEach, describe, expect, it, vi } from 'vitest';
import { deleteFile } from './deletefile.service';

afterEach(() => vi.unstubAllGlobals());

describe('deleteFile', () => {
  it('DELETEs with the path query and accepts 204', async () => {
    const f = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', f);
    await expect(deleteFile('p1', 'docs/a.md')).resolves.toBeUndefined();
    const [url, init] = f.mock.calls[0]! as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p1/knowledge/files?path=docs%2Fa.md');
    expect(init.method).toBe('DELETE');
  });
});
