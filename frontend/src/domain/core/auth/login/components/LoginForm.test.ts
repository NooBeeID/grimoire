import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import LoginForm from './LoginForm.svelte';

describe('LoginForm', () => {
  it('requires both fields client-side', async () => {
    const onsubmit = vi.fn();
    render(LoginForm, { onsubmit });
    await userEvent.click(screen.getByRole('button', { name: 'Masuk' }));
    expect(onsubmit).not.toHaveBeenCalled();
    expect(screen.getAllByText('Wajib diisi')).toHaveLength(2);
  });
  it('submits credentials without checking email format', async () => {
    const onsubmit = vi.fn();
    render(LoginForm, { onsubmit });
    await userEvent.type(screen.getByLabelText('Email'), 'bukan-email');
    await userEvent.type(screen.getByLabelText('Password'), 'rahasia');
    await userEvent.click(screen.getByRole('button', { name: 'Masuk' }));
    expect(onsubmit).toHaveBeenCalledWith({ email: 'bukan-email', password: 'rahasia' });
  });
  it('shows generic invalid message and clears password', async () => {
    const onsubmit = vi.fn();
    const { rerender } = render(LoginForm, { onsubmit });
    await userEvent.type(screen.getByLabelText('Password'), 'salah');
    await rerender({ onsubmit, error: { kind: 'invalid' } });
    expect(screen.getByRole('alert')).toHaveTextContent('Email atau password salah');
    expect(screen.getByLabelText('Password')).toHaveValue('');
    expect(screen.getByLabelText('Password')).toHaveFocus();
  });
  it('locked message uses minutes, with fallback', async () => {
    const onsubmit = vi.fn();
    const { rerender } = render(LoginForm, { onsubmit, error: { kind: 'locked', retryAfterSeconds: 812 } });
    expect(screen.getByRole('alert')).toHaveTextContent('14 menit');
    await rerender({ onsubmit, error: { kind: 'locked', retryAfterSeconds: null } });
    expect(screen.getByRole('alert')).toHaveTextContent('Coba lagi nanti');
  });
  it('disables button during cooldown and shows seconds', () => {
    render(LoginForm, { onsubmit: vi.fn(), cooldownSeconds: 5, error: { kind: 'rateLimited', retryAfterSeconds: 5 } });
    expect(screen.getByRole('button', { name: 'Tunggu 5 dtk' })).toBeDisabled();
  });
  it('shows server field errors and generic message with trace id', () => {
    const { unmount } = render(LoginForm, { onsubmit: vi.fn(), error: { kind: 'fields', fields: { email: 'terlalu panjang' } } });
    expect(screen.getByText('terlalu panjang')).toBeInTheDocument();
    unmount();
    render(LoginForm, { onsubmit: vi.fn(), error: { kind: 'generic', message: 'gagal server', traceId: 'abc' } });
    expect(screen.getByRole('alert')).toHaveTextContent('gagal server');
    expect(screen.getByText('ID jejak: abc')).toBeInTheDocument();
  });
});
