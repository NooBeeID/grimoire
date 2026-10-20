import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { isApiError } from '../../../../../shared/services/http';
import { fetchMe } from './getme.service';

const fetchMock = vi.fn();
const json = (b: unknown, s = 200) => new Response(JSON.stringify(b), { status: s });
const ME = { id: '0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d', email: 'budi@tim.id', name: 'Budi', role: 'developer', must_change_password: false };

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
});
afterEach(() => vi.unstubAllGlobals());

describe('fetchMe', () => {
  it('maps mock response to Identity', async () => {
    fetchMock.mockResolvedValue(json(ME));
    expect(await fetchMe()).toEqual({ userId: ME.id, email: 'budi@tim.id', name: 'Budi', role: 'developer', mustChangePassword: false });
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/auth/me');
  });
  it('401 does not trigger global redirect and surfaces ApiError', async () => {
    history.pushState({}, '', '/x');
    fetchMock.mockResolvedValue(json({ error: { code: 'UNAUTHORIZED', message: 'm' } }, 401));
    const e = await fetchMe().catch((x) => x);
    expect(isApiError(e) && e.status).toBe(401);
    expect(location.pathname).toBe('/x');
  });
  it('rejects malformed body with INVALID_RESPONSE', async () => {
    fetchMock.mockResolvedValue(json({ ...ME, role: 'root' }));
    const e = await fetchMe().catch((x) => x);
    expect(isApiError(e) && e.code).toBe('INVALID_RESPONSE');
  });
});
