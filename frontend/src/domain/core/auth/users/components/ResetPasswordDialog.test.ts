import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { User } from '../types';
import ResetPasswordDialog from './ResetPasswordDialog.svelte';

const U = (o: Partial<User> = {}): User => ({
  id: '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true,
  must_change_password: false, created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null,
  updated_at: '2026-10-21T09:00:00Z', last_login_at: null, invitation: null, ...o,
});

describe('ResetPasswordDialog', () => {
  it('generate mode submits undefined; manual requires password', async () => {
    const onsubmit = vi.fn();
    render(ResetPasswordDialog, { open: true, user: U(), onsubmit, onclose: vi.fn() });
    const btn = () => screen.getAllByRole('button', { name: 'Reset password' }).at(-1)!;
    await userEvent.click(btn());
    expect(onsubmit).toHaveBeenCalledWith(undefined);
    await userEvent.selectOptions(screen.getByLabelText('Password awal'), 'manual');
    await userEvent.click(btn());
    expect(screen.getByText('Wajib diisi')).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('Password'), 'manual-pass-1');
    await userEvent.click(btn());
    expect(onsubmit).toHaveBeenLastCalledWith('manual-pass-1');
  });
  it('shows secret stage', () => {
    render(ResetPasswordDialog, { open: true, user: U(), temporaryPassword: 'tmp-123', onsubmit: vi.fn(), onclose: vi.fn() });
    expect((screen.getByLabelText('Password sementara') as HTMLInputElement).value).toBe('tmp-123');
  });
});
