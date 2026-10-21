import { afterEach, describe, expect, it, vi } from 'vitest';
import { deleteSpec } from './deletespec.service';

afterEach(() => vi.unstubAllGlobals());

describe('deleteSpec', () => {
  it('DELETE with CSRF header and 204 without body', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', f);
    await expect(deleteSpec('p1', 'payment')).resolves.toBeUndefined();
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/api-specs/payment');
    expect(f.mock.calls[0]![1]!.method).toBe('DELETE');
    expect(f.mock.calls[0]![1]!.headers).toMatchObject({ 'X-Requested-With': 'grimoire' });
  });
  it('404 -> ApiError 404', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x', details: null } }), { status: 404 })));
    await expect(deleteSpec('p1', 'x')).rejects.toMatchObject({ status: 404 });
  });
});
