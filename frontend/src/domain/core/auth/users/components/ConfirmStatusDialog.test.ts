import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { User } from '../types';
import ConfirmStatusDialog from './ConfirmStatusDialog.svelte';

const U = (o: Partial<User> = {}): User => ({
  id: '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true,
  must_change_password: false, created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null,
  updated_at: '2026-10-21T09:00:00Z', last_login_at: null, invitation: null, ...o,
});

describe('ConfirmStatusDialog', () => {
  it('deactivate is danger and explains sessions end; shows error', async () => {
    const onconfirm = vi.fn();
    render(ConfirmStatusDialog, { open: true, user: U(), onconfirm, onclose: vi.fn(), error: { kind: 'form', fields: {}, message: 'Minimal harus ada satu super admin aktif' } });
    expect(screen.getByText(/semua sesinya langsung berakhir/)).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveTextContent('super admin aktif');
    await userEvent.click(screen.getByRole('button', { name: 'Nonaktifkan' }));
    expect(onconfirm).toHaveBeenCalled();
  });
  it('activate wording for inactive user', () => {
    render(ConfirmStatusDialog, { open: true, user: U({ is_active: false }), onconfirm: vi.fn(), onclose: vi.fn() });
    expect(screen.getByText(/dapat login kembali/)).toBeInTheDocument();
  });
});

