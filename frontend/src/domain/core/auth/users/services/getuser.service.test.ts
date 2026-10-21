import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchUser } from './getuser.service';

const fetchMock = vi.fn();
const json = (b: unknown, s = 200) => new Response(JSON.stringify(b), { status: s });
const ID = '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c';
const USER = {
  id: ID, email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true, must_change_password: false,
  created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null, updated_at: '2026-10-21T09:00:00Z', last_login_at: null, invitation: null,
};

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
});
afterEach(() => vi.unstubAllGlobals());

describe('fetchUser', () => {
  it('returns a valid user and encodes the id', async () => {
    fetchMock.mockResolvedValue(json(USER));
    const u = await fetchUser(ID);
    expect(u.email).toBe('sari@tim.id');
    expect(fetchMock.mock.calls[0]![0]).toBe(`/api/users/${ID}`);
    fetchMock.mockResolvedValue(json(USER));
    await fetchUser('a/b');
    expect(fetchMock.mock.calls[1]![0]).toBe('/api/users/a%2Fb');
  });
  it('maps wrong role or uuid to INVALID_RESPONSE', async () => {
    fetchMock.mockResolvedValue(json({ ...USER, role: 'owner' }));
    await expect(fetchUser(ID)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
    fetchMock.mockResolvedValue(json({ ...USER, id: 'not-a-uuid' }));
    await expect(fetchUser(ID)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('maps 404 to ApiError 404', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'NOT_FOUND', message: 'x', details: null }, trace_id: 't' }, 404));
    await expect(fetchUser(ID)).rejects.toMatchObject({ status: 404, code: 'NOT_FOUND' });
  });
});
