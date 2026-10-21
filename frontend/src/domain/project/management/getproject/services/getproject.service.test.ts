import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockDetail } from '../mocks/project';
import { getProject } from './getproject.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}

describe('getProject', () => {
  it('encodes the path', async () => {
    const f = stub(200, mockDetail());
    await getProject('a/b');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/a%2Fb');
  });
  it('requires owner_count', async () => {
    const { owner_count: _o, ...rest } = mockDetail();
    stub(200, rest);
    await expect(getProject('x')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('404 becomes ApiError NOT_FOUND', async () => {
    stub(404, { error: { code: 'NOT_FOUND', message: 'project tidak ditemukan', details: null } });
    await expect(getProject('x')).rejects.toMatchObject({ status: 404, code: 'NOT_FOUND' });
  });
});
