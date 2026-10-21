import { afterEach, describe, expect, it, vi } from 'vitest';
import { deleteFolder } from './deletefolder.service';

afterEach(() => vi.unstubAllGlobals());

describe('deleteFolder', () => {
  it('DELETEs /folders?path= and parses {deleted}', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify({ deleted: 3 }), { status: 200 }));
    vi.stubGlobal('fetch', f);
    await expect(deleteFolder('p1', 'docs/auth')).resolves.toEqual({ deleted: 3 });
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/knowledge/folders?path=docs%2Fauth');
  });
  it('never sends an empty path', () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    expect(() => deleteFolder('p1', '')).toThrow();
    expect(f).not.toHaveBeenCalled();
  });
});
