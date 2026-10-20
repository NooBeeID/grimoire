import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ChangePasswordForm from './ChangePasswordForm.svelte';

async function type(cur: string, nw: string, conf: string) {
  if (cur) await userEvent.type(screen.getByLabelText('Password saat ini'), cur);
  if (nw) await userEvent.type(screen.getByLabelText('Password baru'), nw);
  if (conf) await userEvent.type(screen.getByLabelText('Ulangi password baru'), conf);
  await userEvent.click(screen.getByRole('button', { name: 'Simpan password' }));
}

describe('ChangePasswordForm', () => {
  it('requires all fields', async () => {
    const onsubmit = vi.fn();
    render(ChangePasswordForm, { onsubmit });
    await type('', '', '');
    expect(screen.getAllByText('Wajib diisi')).toHaveLength(3);
    expect(onsubmit).not.toHaveBeenCalled();
  });
  it('checks mismatch and same-as-current', async () => {
    const onsubmit = vi.fn();
    const { unmount } = render(ChangePasswordForm, { onsubmit });
    await type('lama-lama', 'baru-baru', 'beda');
    expect(screen.getByText('Password baru tidak sama')).toBeInTheDocument();
    unmount();
    render(ChangePasswordForm, { onsubmit });
    await type('sama-sama', 'sama-sama', 'sama-sama');
    expect(screen.getByText('Password baru harus berbeda dari password saat ini')).toBeInTheDocument();
    expect(onsubmit).not.toHaveBeenCalled();
  });
  it('does not enforce minimum length and submits without confirm', async () => {
    const onsubmit = vi.fn();
    render(ChangePasswordForm, { onsubmit });
    await type('a', 'b', 'b');
    expect(onsubmit).toHaveBeenCalledWith({ current_password: 'a', new_password: 'b' });
  });
  it('shows server field errors and conflict', () => {
    const { unmount } = render(ChangePasswordForm, { onsubmit: vi.fn(), error: { kind: 'fields', fields: { current_password: 'password lama salah' } } });
    expect(screen.getByText('password lama salah')).toBeInTheDocument();
    unmount();
    render(ChangePasswordForm, { onsubmit: vi.fn(), error: { kind: 'conflict' } });
    expect(screen.getByRole('alert')).toHaveTextContent('Muat ulang');
  });
  it('clears fields when resetKey changes', async () => {
    const onsubmit = vi.fn();
    const { rerender } = render(ChangePasswordForm, { onsubmit, resetKey: 0 });
    await userEvent.type(screen.getByLabelText('Password saat ini'), 'abc');
    await rerender({ onsubmit, resetKey: 1 });
    expect(screen.getByLabelText('Password saat ini')).toHaveValue('');
  });
});
