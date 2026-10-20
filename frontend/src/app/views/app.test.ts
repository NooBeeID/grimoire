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
    boot('/');
    expect(screen.getByTestId('home-title')).toHaveTextContent('Selamat datang di Grimoire');
    expect(document.title).toBe('Selamat datang di Grimoire · Grimoire');
  });

  it('renders not found, then navigates home without reload', async () => {
    (window as unknown as { __marker: number }).__marker = 1;
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
});
