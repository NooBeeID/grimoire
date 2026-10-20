import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { logoutRequest } from './logout.service';

const fetchMock = vi.fn();
beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
});
afterEach(() => vi.unstubAllGlobals());

describe('logoutRequest', () => {
  it('POSTs with CSRF header and no body, tolerates 204', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(logoutRequest()).resolves.toBeUndefined();
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/auth/logout');
    expect(init.method).toBe('POST');
    expect(init.headers['X-Requested-With']).toBe('grimoire');
    expect(init.body).toBeUndefined();
  });
  it('401 does not redirect globally', async () => {
    history.pushState({}, '', '/x');
    fetchMock.mockResolvedValue(new Response(JSON.stringify({ error: { code: 'UNAUTHORIZED', message: 'm' } }), { status: 401 }));
    await expect(logoutRequest()).rejects.toMatchObject({ status: 401 });
    expect(location.pathname).toBe('/x');
  });
});
