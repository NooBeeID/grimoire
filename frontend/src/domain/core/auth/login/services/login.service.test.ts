import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { login } from './login.service';

const fetchMock = vi.fn();
const json = (b: unknown, s = 200) => new Response(JSON.stringify(b), { status: s });
const USER = { id: '0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d', email: 'budi@tim.id', name: 'Budi', role: 'developer', must_change_password: true };

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
});
afterEach(() => vi.unstubAllGlobals());

describe('login service', () => {
  it('posts credentials as-is and returns Identity', async () => {
    fetchMock.mockResolvedValue(json({ user: USER }));
    const id = await login({ email: 'Budi@Tim.ID', password: 'rahasia-panjang-1' });
    expect(id).toMatchObject({ userId: USER.id, mustChangePassword: true, role: 'developer' });
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/auth/login');
    expect(JSON.parse(init.body)).toEqual({ email: 'Budi@Tim.ID', password: 'rahasia-panjang-1' });
    expect(init.headers['X-Requested-With']).toBe('grimoire');
  });
  it('401 INVALID_CREDENTIALS surfaces without redirect', async () => {
    history.pushState({}, '', '/login');
    fetchMock.mockResolvedValue(json({ error: { code: 'INVALID_CREDENTIALS', message: 'x', details: null } }, 401));
    await expect(login({ email: 'a', password: 'b' })).rejects.toMatchObject({ code: 'INVALID_CREDENTIALS', status: 401 });
  });
  it('rejects response missing user', async () => {
    fetchMock.mockResolvedValue(json({}));
    await expect(login({ email: 'a', password: 'b' })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});
