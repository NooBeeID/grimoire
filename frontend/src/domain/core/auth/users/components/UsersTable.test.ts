import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import type { User } from '../types';
import UsersTable from './UsersTable.svelte';

const U = (o: Partial<User> = {}): User => ({
  id: '7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', email: 'sari@tim.id', name: 'Sari', role: 'developer', is_active: true,
  must_change_password: false, created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null,
  updated_at: '2026-10-21T09:00:00Z', last_login_at: null, invitation: null, ...o,
});

describe('UsersTable', () => {
  it('renders status text, badge, never-logged-in, self label', () => {
    const me = U({ must_change_password: true });
    render(UsersTable, { users: [me, U({ id: '2d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', name: 'Dewi', is_active: false })], selfId: me.id, onedit: vi.fn(), onstatus: vi.fn(), onreset: vi.fn() });
    expect(screen.getByText('(Anda)')).toBeInTheDocument();
    expect(screen.getByText('Wajib ganti password')).toBeInTheDocument();
    expect(screen.getByText('Nonaktif')).toBeInTheDocument();
    expect(screen.getAllByText('Belum pernah')).toHaveLength(2);
  });
});
