import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { User } from '../types';
import UserRowActions from './UserRowActions.svelte';

const U = (o: Partial<User> = {}): User => ({
  id: '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true,
  must_change_password: false, created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null,
  updated_at: '2026-10-21T09:00:00Z', last_login_at: null, invitation: null, ...o,
});

describe('UserRowActions', () => {
  it('disables deactivate and reset for self but not edit', async () => {
    const onedit = vi.fn();
    render(UserRowActions, { user: U(), isSelf: true, onedit, onstatus: vi.fn(), onreset: vi.fn() });
    expect(screen.getByRole('button', { name: 'Nonaktifkan' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Reset password' })).toBeDisabled();
    await userEvent.click(screen.getByRole('button', { name: 'Ubah' }));
    expect(onedit).toHaveBeenCalled();
  });
  it('shows Aktifkan for inactive user', () => {
    render(UserRowActions, { user: U({ is_active: false }), onedit: vi.fn(), onstatus: vi.fn(), onreset: vi.fn() });
    expect(screen.getByRole('button', { name: 'Aktifkan' })).toBeEnabled();
  });
});

