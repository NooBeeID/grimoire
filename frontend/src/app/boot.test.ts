import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, request, setUnauthorizedHandler } from '../shared/services/http';
import { resetSession, session, setIdentity } from '../shared/stores/session';
import { toast, toasts } from '../shared/stores/toast';
import { boot, bootState, installAuthHandlers, logoutAndLeave } from './boot';

const DEV = { userId: '1', email: 'a@b.c', name: 'A', role: 'developer', mustChangePassword: false } as const;

beforeEach(() => {
  resetSession();
  toast.clear();
  history.replaceState({}, '', '/');
});
afterEach(() => {
  setUnauthorizedHandler(null);
  vi.unstubAllGlobals();
});

describe('boot', () => {
  it('goes loading -> ready, or failed with error', async () => {
    const p = boot({ loadSession: async () => {} });
    expect(get(bootState).kind).toBe('loading');
    await p;
    expect(get(bootState).kind).toBe('ready');
    const err = new Error('x');
    await boot({ loadSession: async () => Promise.reject(err) });
    expect(get(bootState)).toEqual({ kind: 'failed', error: err });
    expect(get(session).status).toBe('unknown');
  });
});

describe('installAuthHandlers', () => {
  it('401 resets identity and navigates to login with next', async () => {
    setIdentity(DEV);
    history.replaceState({}, '', '/admin/users?x=1');
    installAuthHandlers();
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'UNAUTHORIZED', message: 'm' } }), { status: 401 })));
    await request('GET', '/api/users').catch(() => {});
    expect(get(session).status).toBe('anonymous');
    expect(location.pathname).toBe('/login');
    expect(location.search).toBe('?next=%2Fadmin%2Fusers%3Fx%3D1');
  });
});

describe('logoutAndLeave', () => {
  it('on success navigates to /login without next and clears identity', async () => {
    setIdentity(DEV);
    history.replaceState({}, '', '/profile');
    await logoutAndLeave({ performLogout: async () => ({ ok: true }) });
    expect(location.pathname + location.search).toBe('/login');
    expect(get(session).status).toBe('anonymous');
  });
  it('on failure toasts and keeps session and page', async () => {
    setIdentity(DEV);
    history.replaceState({}, '', '/profile');
    await logoutAndLeave({ performLogout: async () => ({ ok: false, error: new ApiError({ status: 500, code: 'INTERNAL', message: 'gagal' }) }) });
    expect(location.pathname).toBe('/profile');
    expect(get(session).status).toBe('authenticated');
    expect(get(toasts)[0]?.message).toBe('gagal');
  });
});
