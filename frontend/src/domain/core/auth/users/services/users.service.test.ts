import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createUser, listUsers, resetUserPassword, setUserStatus, updateUser } from './users.service';
import { fetchUser } from './getuser.service';

const fetchMock = vi.fn();
const json = (b: unknown, s = 200) => new Response(JSON.stringify(b), { status: s });
const ID = '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c';
const USER = {
  id: ID, email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true, must_change_password: true,
  created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null, updated_at: '2026-10-21T09:00:00Z', last_login_at: null,
};
const call = () => fetchMock.mock.calls.at(-1)! as [string, RequestInit & { headers: Record<string, string> }];

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
});
afterEach(() => vi.unstubAllGlobals());

describe('listUsers', () => {
  it('drops empty and whitespace-only filters', async () => {
    fetchMock.mockResolvedValue(json({ items: [USER], total: 1, limit: 20, offset: 0 }));
    const r = await listUsers({ role: '', status: '', q: '   ', limit: 20, offset: 0 });
    expect(r.total).toBe(1);
    expect(call()[0]).toBe('/api/users?limit=20&offset=0');
  });
  it('sends filters', async () => {
    fetchMock.mockResolvedValue(json({ items: [], total: 0, limit: 20, offset: 40 }));
    await listUsers({ role: 'developer', status: 'active', q: ' sari ', limit: 20, offset: 40 });
    expect(call()[0]).toBe('/api/users?role=developer&status=active&q=sari&limit=20&offset=40');
  });
  it('rejects invalid body', async () => {
    fetchMock.mockResolvedValue(json({ items: [{ ...USER, role: 'x' }], total: 1, limit: 20, offset: 0 }));
    await expect(listUsers({ role: '', status: '', q: '', limit: 20, offset: 0 })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
});

describe('createUser', () => {
  it('omits password key in generate mode', async () => {
    fetchMock.mockResolvedValue(json({ user: USER, temporary_password: 'Kq7mZp3RtX9aHw2N' }, 201));
    const r = await createUser({ email: 'sari@tim.id', name: 'Sari', role: 'developer' });
    expect(r.temporary_password).toBe('Kq7mZp3RtX9aHw2N');
    expect(JSON.parse(call()[1].body as string)).toEqual({ email: 'sari@tim.id', name: 'Sari', role: 'developer' });
    expect(call()[1].headers['X-Requested-With']).toBe('grimoire');
  });
  it('sends manual password', async () => {
    fetchMock.mockResolvedValue(json({ user: USER, temporary_password: null }, 201));
    await createUser({ email: 'a@b.c', name: 'A', role: 'developer', password: 'pw-manual-1' });
    expect(JSON.parse(call()[1].body as string).password).toBe('pw-manual-1');
  });
});

describe('updateUser / status / reset / get', () => {
  it('PATCH sends only given fields; empty patch throws', async () => {
    fetchMock.mockImplementation(async () => json(USER));
    await updateUser(ID, { name: 'Baru' });
    expect(call()[1].method).toBe('PATCH');
    expect(JSON.parse(call()[1].body as string)).toEqual({ name: 'Baru' });
    expect(() => updateUser(ID, {})).toThrow();
  });
  it('status posts to activate/deactivate without body', async () => {
    fetchMock.mockImplementation(async () => json(USER));
    await setUserStatus(ID, false);
    expect(call()[0]).toBe(`/api/users/${ID}/deactivate`);
    expect(call()[1].body).toBeUndefined();
    await setUserStatus(ID, true);
    expect(call()[0]).toBe(`/api/users/${ID}/activate`);
  });
  it('reset sends {} or password', async () => {
    fetchMock.mockImplementation(async () => json({ user: USER, temporary_password: "x" }));
    await resetUserPassword(ID);
    expect(call()[1].body).toBe('{}');
    await resetUserPassword(ID, 'manual-pass');
    expect(JSON.parse(call()[1].body as string)).toEqual({ password: 'manual-pass' });
  });
  it('encodes ids and validates detail', async () => {
    fetchMock.mockImplementation(async () => json(USER));
    await fetchUser('a/b');
    expect(call()[0]).toBe('/api/users/a%2Fb');
  });
});
