import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockDetail } from '../mocks/detail-3.0';
import { getSpec } from './getspec.service';

afterEach(() => vi.unstubAllGlobals());
const stub = (status: number, body: unknown) => {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
};

describe('getSpec', () => {
  it('GET with encoded project and name, validated by schema', async () => {
    const f = stub(200, mockDetail());
    const r = await getSpec('a/b', 'pay ment');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/a%2Fb/api-specs/pay%20ment');
    expect(r.endpoints).toHaveLength(5);
  });
  it('invalid body -> INVALID_RESPONSE; 404 -> ApiError 404', async () => {
    stub(200, { spec: {}, endpoints: [], document: {} });
    await expect(getSpec('p', 'x')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
    stub(404, { error: { code: 'NOT_FOUND', message: 'spec tidak ditemukan', details: null } });
    await expect(getSpec('p', 'x')).rejects.toMatchObject({ status: 404 });
  });
});
