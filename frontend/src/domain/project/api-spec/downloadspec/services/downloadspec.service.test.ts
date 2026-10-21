import { afterEach, describe, expect, it, vi } from 'vitest';
import { downloadSpec } from './downloadspec.service';

afterEach(() => vi.unstubAllGlobals());

describe('downloadSpec', () => {
  it('fetches raw text and derives the file name from name and format', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response('openapi: 3.0.3', { status: 200 }));
    vi.stubGlobal('fetch', f);
    const r = await downloadSpec('p1', 'payment', 'yaml');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/api-specs/payment/raw');
    expect(r.fileName).toBe('payment.yaml');
    expect(await r.blob.text()).toBe('openapi: 3.0.3');
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 200 })));
    expect((await downloadSpec('p1', 'auth', 'json')).fileName).toBe('auth.json');
  });
  it('404 -> ApiError 404', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x', details: null } }), { status: 404 })));
    await expect(downloadSpec('p1', 'x', 'yaml')).rejects.toMatchObject({ status: 404 });
  });
});
