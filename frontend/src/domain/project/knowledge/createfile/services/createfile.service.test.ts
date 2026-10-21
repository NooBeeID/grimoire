import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockFile } from '../../base';
import { createFile } from './createfile.service';

afterEach(() => vi.unstubAllGlobals());

describe('createFile', () => {
  it('POSTs {path, content} with CSRF header and validates the response', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(mockFile('docs/a.md')), { status: 201 }));
    vi.stubGlobal('fetch', f);
    const r = await createFile('p1', { path: 'docs/a.md', content: '# a' });
    const [url, init] = f.mock.calls[0]! as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p1/knowledge/files');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body as string)).toEqual({ path: 'docs/a.md', content: '# a' });
    expect((init.headers as Record<string, string>)['X-Requested-With']).toBe('grimoire');
    expect(r.path).toBe('docs/a.md');
  });
});
