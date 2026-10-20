import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { changePassword } from './changepassword.service';

const fetchMock = vi.fn();
beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
});
afterEach(() => vi.unstubAllGlobals());

describe('changePassword', () => {
  it('posts body with CSRF header and accepts 204', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(changePassword({ current_password: 'a', new_password: 'b' })).resolves.toBeUndefined();
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe('/api/auth/change-password');
    expect(JSON.parse(init.body)).toEqual({ current_password: 'a', new_password: 'b' });
    expect(init.headers['X-Requested-With']).toBe('grimoire');
  });
  it('surfaces 400 field details', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ error: { code: 'INVALID_ARGUMENT', message: 'm', details: { fields: { current_password: 'password lama salah' } } } }), { status: 400 }),
    );
    await expect(changePassword({ current_password: 'a', new_password: 'b' })).rejects.toMatchObject({ code: 'INVALID_ARGUMENT' });
  });
});
