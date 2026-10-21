import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockFileResult, mockUploadResult, mockValidateResult } from '../mocks/results';
import { uploadSpecs, validateSpecs } from './uploadspecs.service';

afterEach(() => vi.unstubAllGlobals());
function stub(body: unknown, status = 200) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
const files = [{ file: new File(['openapi: 3.0.3'], 'payment.yaml'), path: 'specs/payment.yaml' }];

describe('uploadSpecs / validateSpecs', () => {
  it('POST multipart upload with CSRF header, path as part filename, no manual content-type', async () => {
    const f = stub(mockUploadResult());
    const r = await uploadSpecs('p1', files);
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe('/api/projects/p1/api-specs/upload');
    expect(init!.headers).toMatchObject({ 'X-Requested-With': 'grimoire' });
    expect(init!.headers).not.toHaveProperty('Content-Type');
    const form = init!.body as FormData;
    expect((form.get('files') as File).name).toBe('specs/payment.yaml');
    expect(form.has('name')).toBe(false);
    expect(r.summary.created).toBe(1);
  });
  it('adds name only when given', async () => {
    const f = stub(mockValidateResult([mockFileResult({ status: 'valid' })]));
    await validateSpecs('p1', files, 'payment-v2');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/api-specs/validate');
    expect((f.mock.calls[0]![1]!.body as FormData).get('name')).toBe('payment-v2');
  });
  it('invalid response -> INVALID_RESPONSE', async () => {
    stub({ results: [{ file: 1 }] });
    await expect(uploadSpecs('p1', files)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
