import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockDetail } from '../mocks/detail-3.0';
import { mockDetailSwagger2 } from '../mocks/detail-swagger2';
import * as swagger2 from '../model/swagger2';
import { getSpec } from './getspec.service';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
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
  it('memetakan dokumen 2.0 sebelum dikembalikan', async () => {
    stub(200, mockDetailSwagger2());
    const r = await getSpec('p', 'hr-platform');
    const doc = r.document as Record<string, any>;
    expect(doc.openapi).toBe('3.0.3');
    expect(doc.components.schemas.Employee).toBeDefined();
    expect(doc.swagger).toBeUndefined();
    expect(r.spec.openapi_version).toBe('2.0');
    expect(r.endpoints).toEqual(mockDetailSwagger2().endpoints);
  });
  it('membiarkan dokumen 3.x identik', async () => {
    stub(200, mockDetail());
    const r = await getSpec('p', 'payment');
    expect(r.document).toEqual(mockDetail().document);
  });
  it('swagger angka 2 diterima; "1.2" -> INVALID_RESPONSE', async () => {
    stub(200, mockDetailSwagger2({ document: { swagger: 2, info: { title: 't', version: '1' } } }));
    await expect(getSpec('p', 'x')).resolves.toBeDefined();
    stub(200, mockDetailSwagger2({ document: { swagger: '1.2', info: { title: 't', version: '1' } } as never }));
    await expect(getSpec('p', 'x')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('adapter melempar -> INVALID_RESPONSE', async () => {
    stub(200, mockDetailSwagger2());
    vi.spyOn(swagger2, 'adaptSwagger2').mockImplementation(() => {
      throw new Error('bug');
    });
    await expect(getSpec('p', 'x')).rejects.toMatchObject({ code: 'INVALID_RESPONSE', status: 0 });
  });
});
