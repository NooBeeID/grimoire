import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockFile } from '../../base';
import { moveFile } from './movefile.service';

afterEach(() => vi.unstubAllGlobals());

describe('moveFile', () => {
  it('POSTs {from, to} to /files/move', async () => {
    const f = vi.fn(async () => new Response(JSON.stringify(mockFile('docs/b.md')), { status: 200 }));
    vi.stubGlobal('fetch', f);
    const r = await moveFile('p1', { from: 'docs/a.md', to: 'docs/b.md' });
    const [url, init] = f.mock.calls[0]! as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p1/knowledge/files/move');
    expect(JSON.parse(init.body as string)).toEqual({ from: 'docs/a.md', to: 'docs/b.md' });
    expect(r.path).toBe('docs/b.md');
  });
});
