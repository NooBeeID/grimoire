import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { User } from '../types';
import EditUserDialog from './EditUserDialog.svelte';

const U = (o: Partial<User> = {}): User => ({
  id: '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true,
  must_change_password: false, created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null,
  updated_at: '2026-10-21T09:00:00Z', last_login_at: null, ...o,
});

describe('EditUserDialog', () => {
  it('save disabled until changed; sends only diff', async () => {
    const onsubmit = vi.fn();
    render(EditUserDialog, { open: true, user: U(), onsubmit, onclose: vi.fn() });
    expect(screen.getByRole('button', { name: 'Simpan' })).toBeDisabled();
    const name = screen.getByLabelText('Nama');
    await userEvent.clear(name);
    await userEvent.type(name, 'Sari W.');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(onsubmit).toHaveBeenCalledWith({ name: 'Sari W.' });
  });
  it('locks role for self', () => {
    render(EditUserDialog, { open: true, user: U(), isSelf: true, onsubmit: vi.fn(), onclose: vi.fn() });
    expect(screen.getByLabelText('Role')).toBeDisabled();
    expect(screen.getByText('Tidak bisa mengubah role akun sendiri')).toBeInTheDocument();
  });
  it('shows spinner, not-found and error states', () => {
    const { unmount } = render(EditUserDialog, { open: true, loading: true, onsubmit: vi.fn(), onclose: vi.fn() });
    expect(screen.getByRole('status')).toBeInTheDocument();
    unmount();
    render(EditUserDialog, { open: true, loadError: { kind: 'notFound' }, onsubmit: vi.fn(), onclose: vi.fn() });
    expect(screen.getByRole('alert')).toHaveTextContent('User tidak ditemukan');
  });
});

