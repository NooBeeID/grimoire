import { afterEach, describe, expect, it, vi } from 'vitest';
import { renameSpec } from './renamespec.service';

afterEach(() => vi.unstubAllGlobals());
const spec = {
  name: 'payment-v2', title: 'Payment API', openapi_version: '3.0.3', api_version: '1.4.0', format: 'yaml', source_path: 'specs/payment.yaml',
  size_bytes: 40211, endpoint_count: 2, created_at: '2026-10-23T09:00:00Z', created_by_label: 'Budi', updated_at: '2026-10-24T10:00:00Z', updated_by_label: 'Sari',
};

describe('renameSpec', () => {
  it('PATCH with JSON body and CSRF header; returns the spec', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify({ spec }), { status: 200 }));
    vi.stubGlobal('fetch', f);
    const r = await renameSpec('p1', 'payment', 'payment-v2');
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe('/api/projects/p1/api-specs/payment');
    expect(init!.method).toBe('PATCH');
    expect(init!.body).toBe('{"name":"payment-v2"}');
    expect(init!.headers).toMatchObject({ 'X-Requested-With': 'grimoire' });
    expect(r.name).toBe('payment-v2');
  });
  it('409 NAME_TAKEN surfaces as ApiError with reason', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'CONFLICT', message: 'dipakai', details: { reason: 'NAME_TAKEN' } } }), { status: 409 })));
    await expect(renameSpec('p1', 'a', 'b')).rejects.toMatchObject({ status: 409, details: { reason: 'NAME_TAKEN' } });
  });
});
