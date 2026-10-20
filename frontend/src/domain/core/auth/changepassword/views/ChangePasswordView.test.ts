import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { resetSession, session, setIdentity } from '../../../../../shared/stores/session';
import { toast, toasts } from '../../../../../shared/stores/toast';
import ChangePasswordView from './ChangePasswordView.svelte';

const fetchMock = vi.fn();
const json = (b: unknown, s: number) => new Response(JSON.stringify(b), { status: s });

async function fill() {
  await userEvent.type(screen.getByLabelText('Password saat ini'), 'sementara-1');
  await userEvent.type(screen.getByLabelText('Password baru'), 'baru-sangat-rahasia');
  await userEvent.type(screen.getByLabelText('Ulangi password baru'), 'baru-sangat-rahasia');
  await userEvent.click(screen.getByRole('button', { name: 'Simpan password' }));
}

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
  toast.clear();
  setIdentity({ userId: '1', email: 'a@b.c', name: 'A', role: 'developer', mustChangePassword: true });
});
afterEach(() => {
  vi.unstubAllGlobals();
  resetSession();
});

describe('ChangePasswordView', () => {
  it('forced mode shows description and logout link', async () => {
    const onlogout = vi.fn();
    render(ChangePasswordView, { mode: 'forced', onlogout });
    expect(screen.getByText(/wajib mengganti password sementara/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Keluar' }));
    expect(onlogout).toHaveBeenCalled();
  });
  it('success clears flag, toasts and resets form', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    render(ChangePasswordView, { mode: 'profile' });
    await fill();
    await vi.waitFor(() => expect(get(session).identity?.mustChangePassword).toBe(false));
    expect(get(toasts)[0]?.message).toContain('Password diganti');
    await vi.waitFor(() => expect(screen.getByLabelText('Password saat ini')).toHaveValue(''));
  });
  it('shows server field error', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'INVALID_ARGUMENT', message: 'm', details: { fields: { current_password: 'password lama salah' } } } }, 400));
    render(ChangePasswordView, { mode: 'profile' });
    await fill();
    expect(await screen.findByText('password lama salah')).toBeInTheDocument();
    expect(get(session).identity?.mustChangePassword).toBe(true);
  });
  it('shows conflict message', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'CONFLICT', message: 'm' } }, 409));
    render(ChangePasswordView, { mode: 'profile' });
    await fill();
    expect(await screen.findByRole('alert')).toHaveTextContent('Muat ulang');
  });
  it('rate limit starts cooldown', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'RATE_LIMITED', message: 'm', details: { retry_after_seconds: 30 } } }, 429));
    render(ChangePasswordView, { mode: 'profile' });
    await fill();
    expect(await screen.findByRole('button', { name: /Tunggu 30 dtk/ })).toBeDisabled();
  });
});
