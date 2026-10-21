import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockKey, mockKeyList } from '../mocks/keys';
import { listKeys } from './listkeys.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}

describe('listKeys', () => {
  it('GET with encoded id, no CSRF header', async () => {
    const f = stub(200, mockKeyList(mockKey(1)));
    const r = await listKeys('a/b');
    expect(f.mock.calls[0]![0]).toBe('/api/projects/a%2Fb/api-keys');
    expect(f.mock.calls[0]![1]!.headers).not.toHaveProperty('X-Requested-With');
    expect(r.items).toHaveLength(1);
  });
  it('invalid response -> INVALID_RESPONSE', async () => {
    stub(200, { items: [{ id: 1 }] });
    await expect(listKeys('p')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('404 envelope -> ApiError 404', async () => {
    stub(404, { error: { code: 'NOT_FOUND', message: 'project tidak ditemukan', details: null } });
    await expect(listKeys('p')).rejects.toMatchObject({ status: 404, message: 'project tidak ditemukan' });
  });
});
