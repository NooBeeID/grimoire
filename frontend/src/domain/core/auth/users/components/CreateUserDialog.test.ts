import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { User } from '../types';
import CreateUserDialog from './CreateUserDialog.svelte';

const U = (o: Partial<User> = {}): User => ({
  id: '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true,
  must_change_password: false, created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null,
  updated_at: '2026-10-21T09:00:00Z', last_login_at: null, invitation: null, ...o,
});

describe('CreateUserDialog', () => {
  it('validates required fields and manual password', async () => {
    const onsubmit = vi.fn();
    render(CreateUserDialog, { open: true, onsubmit, onclose: vi.fn() });
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(screen.getAllByText('Wajib diisi')).toHaveLength(2);
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c');
    await userEvent.type(screen.getByLabelText('Nama'), 'A');
    await userEvent.selectOptions(screen.getByLabelText('Password awal'), 'manual');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(onsubmit).not.toHaveBeenCalled();
    expect(screen.getByText('Wajib diisi')).toBeInTheDocument();
  });
  it('generate mode omits password; default role developer', async () => {
    const onsubmit = vi.fn();
    render(CreateUserDialog, { open: true, onsubmit, onclose: vi.fn() });
    await userEvent.type(screen.getByLabelText('Email'), 'a@b.c');
    await userEvent.type(screen.getByLabelText('Nama'), 'A');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(onsubmit).toHaveBeenCalledWith({ email: 'a@b.c', name: 'A', role: 'developer' });
  });
  it('shows the secret stage when a temporary password exists', () => {
    render(CreateUserDialog, { open: true, onsubmit: vi.fn(), onclose: vi.fn(), result: { user: U(), temporaryPassword: 'Kq7mZp3RtX9aHw2N' } });
    expect(screen.getByRole('button', { name: 'Saya sudah menyimpan password' })).toBeInTheDocument();
    expect((screen.getByLabelText('Password sementara') as HTMLInputElement).value).toBe('Kq7mZp3RtX9aHw2N');
  });
  it('shows server field error', () => {
    render(CreateUserDialog, { open: true, onsubmit: vi.fn(), onclose: vi.fn(), error: { kind: 'form', fields: { email: 'Email sudah terdaftar' } } });
    expect(screen.getByText('Email sudah terdaftar')).toBeInTheDocument();
  });
});

