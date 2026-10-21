import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockSpec, mockSpecList } from '../mocks/specs';
import { listSpecs } from './listspecs.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}

describe('listSpecs', () => {
  it('GET with encoded id, no CSRF header', async () => {
    const f = stub(200, mockSpecList());
    const r = await listSpecs('a/b');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/a%2Fb/api-specs');
    expect(f.mock.calls[0]![1]!.headers).not.toHaveProperty('X-Requested-With');
    expect(r.items).toHaveLength(2);
    expect(r.limit).toBe(100);
  });
  it('accepts null source_path and labels', async () => {
    stub(200, mockSpecList([mockSpec({ source_path: null, created_by_label: null, updated_by_label: null })]));
    await expect(listSpecs('p')).resolves.toMatchObject({ total: 1 });
  });
  it('invalid response -> INVALID_RESPONSE', async () => {
    stub(200, { items: [mockSpec({ format: 'xml' as never })], total: 1, limit: 100 });
    await expect(listSpecs('p')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('missing limit -> INVALID_RESPONSE', async () => {
    stub(200, { items: [], total: 0 });
    await expect(listSpecs('p')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('404 envelope -> ApiError 404', async () => {
    stub(404, { error: { code: 'NOT_FOUND', message: 'project tidak ditemukan', details: null } });
    await expect(listSpecs('p')).rejects.toMatchObject({ status: 404 });
  });
});
