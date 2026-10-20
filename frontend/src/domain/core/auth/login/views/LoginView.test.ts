import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { resetSession, session } from '../../../../../shared/stores/session';
import LoginView from './LoginView.svelte';

const fetchMock = vi.fn();
const json = (b: unknown, s = 200, h: Record<string, string> = {}) => new Response(JSON.stringify(b), { status: s, headers: h });
const USER = { id: '0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d', email: 'budi@tim.id', name: 'Budi', role: 'developer', must_change_password: false };

async function fill() {
  await userEvent.type(screen.getByLabelText('Email'), 'budi@tim.id');
  await userEvent.type(screen.getByLabelText('Password'), 'rahasia-panjang-1');
  await userEvent.click(screen.getByRole('button', { name: 'Masuk' }));
}

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
  resetSession();
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('LoginView', () => {
  it('sets identity on success and does not navigate itself', async () => {
    history.pushState({}, '', '/login');
    fetchMock.mockResolvedValue(json({ user: USER }));
    render(LoginView);
    await fill();
    await vi.waitFor(() => expect(get(session).status).toBe('authenticated'));
    expect(location.pathname).toBe('/login');
  });
  it('shows identical generic message on 401', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'INVALID_CREDENTIALS', message: 'x', details: null } }, 401));
    render(LoginView);
    await fill();
    expect(await screen.findByRole('alert')).toHaveTextContent('Email atau password salah');
    expect(get(session).status).toBe('unknown');
  });
  it('shows locked minutes', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'ACCOUNT_LOCKED', message: 'x', details: { retry_after_seconds: 600 } } }, 429));
    render(LoginView);
    await fill();
    expect(await screen.findByRole('alert')).toHaveTextContent('10 menit');
    expect(screen.getByRole('button', { name: 'Masuk' })).toBeEnabled();
  });
  it('rate limited starts a countdown that re-enables the button', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'RATE_LIMITED', message: 'x', details: { retry_after_seconds: 2 } } }, 429));
    render(LoginView);
    await fill();
    expect(await screen.findByRole('button', { name: /Tunggu 2 dtk/ })).toBeDisabled();
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Masuk' })).toBeEnabled(), { timeout: 3500 });
  });
  it('shows generic error with trace id', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'INTERNAL', message: 'server rusak' }, trace_id: 'tr-1' }, 500));
    render(LoginView);
    await fill();
    expect(await screen.findByText('server rusak')).toBeInTheDocument();
    expect(screen.getByText('ID jejak: tr-1')).toBeInTheDocument();
  });
});
