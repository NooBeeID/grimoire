import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockCredentialList } from '../mocks/credentials';
import { listCredentials } from './listcredentials.service';

afterEach(() => vi.unstubAllGlobals());
function stub(status: number, body: unknown) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}

describe('listCredentials', () => {
  it('GET without CSRF header and forwards the signal', async () => {
    const f = stub(200, mockCredentialList());
    const ctrl = new AbortController();
    const r = await listCredentials(ctrl.signal);
    expect(f.mock.calls[0]![0]).toBe('/api/me/git-credentials');
    expect(f.mock.calls[0]![1]!.headers).not.toHaveProperty('X-Requested-With');
    expect(f.mock.calls[0]![1]!.signal).toBe(ctrl.signal);
    expect(r.items).toHaveLength(2);
  });
  it('invalid response -> INVALID_RESPONSE', async () => {
    stub(200, { enabled: true, allowed_gitlab_hosts: [], items: [{ provider: 'github' }] });
    await expect(listCredentials()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('a token field in the response is rejected', async () => {
    const body = mockCredentialList();
    (body.items[0] as Record<string, unknown>).token = 'ghp_x';
    stub(200, body);
    await expect(listCredentials()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
