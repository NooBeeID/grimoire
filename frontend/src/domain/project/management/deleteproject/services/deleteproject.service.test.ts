import { afterEach, describe, expect, it, vi } from 'vitest';
import { confirmMatches, deleteProject } from './deleteproject.service';

afterEach(() => vi.unstubAllGlobals());

describe('confirmMatches', () => {
  it('exact only', () => {
    expect(confirmMatches('Payments', 'Payments')).toBe(true);
    for (const s of ['payments', 'Payments ', '']) expect(confirmMatches(s, 'Payments')).toBe(false);
  });
});

describe('deleteProject', () => {
  it('DELETEs with confirm_name body and CSRF header; 204 -> undefined', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', f);
    await expect(deleteProject('abc', 'Payments')).resolves.toBeUndefined();
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe('/api/projects/abc');
    expect(init!.method).toBe('DELETE');
    expect(JSON.parse(init!.body as string)).toEqual({ confirm_name: 'Payments' });
    expect((init!.headers as Record<string, string>)['X-Requested-With']).toBe('grimoire');
  });
});
