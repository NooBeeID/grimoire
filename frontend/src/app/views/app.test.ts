import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '../App.svelte';
import { notFoundRoute, routes } from '../routes/index';
import { startRouter, type RouteDef } from '../routes/router';
import { resetSession, setIdentity } from '../../shared/stores/session';
import { setLocale } from '../../shared/stores/i18n';
import PendingView from './PendingView.svelte';
import Thrower from './Thrower.svelte';

const DEV = { userId: '1', email: 'a@b.c', name: 'Budi', role: 'developer', mustChangePassword: false } as const;
const testRoutes: RouteDef[] = [
  ...routes,
  { path: '/secret', view: PendingView, meta: { auth: true } },
  { path: '/boom', view: Thrower, meta: { layout: 'blank' } },
];

let stop: () => void;
function boot(path: string) {
  history.replaceState({}, '', path);
  stop = startRouter(testRoutes, notFoundRoute);
  return render(App);
}

beforeEach(() => {
  setLocale('id');
  resetSession();
});
afterEach(() => stop());

describe('App', () => {
  it('renders home with id text and title', () => {
    setIdentity(DEV);
    boot('/');
    expect(screen.getByTestId('home-title')).toHaveTextContent('Selamat datang di Grimoire');
    expect(document.title).toBe('Selamat datang di Grimoire · Grimoire');
  });

  it('renders not found, then navigates home without reload', async () => {
    (window as unknown as { __marker: number }).__marker = 1;
    setIdentity(DEV);
    boot('/zzz');
    expect(screen.getByTestId('notfound-title')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('link', { name: 'Kembali ke beranda' }));
    expect(await screen.findByTestId('home-title')).toBeInTheDocument();
    expect((window as unknown as { __marker?: number }).__marker).toBe(1);
  });

  it('auth route: pending while unknown, redirects when anonymous', async () => {
    boot('/secret');
    expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
    setIdentity(null);
    await vi.waitFor(() => expect(window.location.pathname).toBe('/login'));
    expect(window.location.search).toBe('?next=%2Fsecret');
  });

  it('boundary shows fallback and keeps toast host', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    boot('/boom');
    expect(await screen.findByText('Terjadi kesalahan tak terduga')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Muat ulang' })).toBeInTheDocument();
    expect(document.querySelector('[aria-live="polite"]')).not.toBeNull();
  });

  it('home greets the user', () => {
    setIdentity(DEV);
    boot('/');
    expect(screen.getByText('Halo, Budi')).toBeInTheDocument();
  });

  it('anonymous at / is sent to login which renders the form', async () => {
    setIdentity(null);
    boot('/');
    expect(await screen.findByRole('button', { name: 'Masuk' })).toBeInTheDocument();
    expect(window.location.pathname).toBe('/login');
  });

  it('authenticated user on /login is redirected to next', async () => {
    setIdentity(DEV);
    boot('/login?next=%2Fprofile');
    await vi.waitFor(() => expect(window.location.pathname).toBe('/profile'));
    expect(screen.getByTestId('profile-card')).toHaveTextContent('Budi');
  });

  it('forced password change redirects every auth route', async () => {
    setIdentity({ ...DEV, mustChangePassword: true });
    boot('/profile');
    await vi.waitFor(() => expect(window.location.pathname).toBe('/change-password'));
    expect(window.location.search).toBe('?next=%2Fprofile');
    expect(screen.getByRole('button', { name: 'Keluar' })).toBeInTheDocument();
  });

  it('users menu and route only for super_admin', async () => {
    setIdentity(DEV);
    boot('/');
    expect(screen.queryByRole('link', { name: 'Users' })).toBeNull();
    stop();
    document.body.innerHTML = '';
    setIdentity({ ...DEV, role: 'super_admin' });
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ items: [], total: 0, limit: 20, offset: 0 }), { status: 200 })));
    boot('/');
    expect(screen.getByRole('link', { name: 'Users' })).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it('shows session error with retry when bootstrap failed, not login', async () => {
    const mod = await import('../boot');
    await mod.boot({ loadSession: async () => Promise.reject(new Error('x')) });
    boot('/');
    expect(await screen.findByText('Gagal memuat sesi')).toBeInTheDocument();
    await mod.boot({ loadSession: async () => {} });
  });
});

