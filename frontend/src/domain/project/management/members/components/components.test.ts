import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { memberId, mockMember } from '../mocks/members';
import MembersTable from './MembersTable.svelte';

const ME = memberId(1);
const items = [
  mockMember(1, { name: 'Ana', role: 'owner' }),
  mockMember(2, { name: 'Budi', role: 'member' }),
  mockMember(3, { name: 'Citra', role: 'owner' }),
];
const props = (accessLevel: 'owner' | 'member' | 'admin', over: Record<string, unknown> = {}) => ({
  items,
  currentUserId: accessLevel === 'admin' ? 'not-in-list' : ME,
  accessLevel,
  onrequestrole: vi.fn(),
  onrequestremove: vi.fn(),
  onrequestleave: vi.fn(),
  ...over,
});

describe('MembersTable', () => {
  it('T03-AC2: member sees only Keluar on own row', () => {
    render(MembersTable, { ...props('member'), items: [mockMember(1, { name: 'Ana' }), mockMember(2, { name: 'Budi' })] });
    expect(screen.queryByRole('button', { name: /^Keluarkan/ })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Jadikan owner' })).toBeNull();
    expect(screen.getAllByRole('button', { name: 'Keluar' })).toHaveLength(1);
  });
  it('owner: others get role + remove; self gets leave + make member', async () => {
    const p = props('owner');
    render(MembersTable, p);
    expect(screen.getAllByRole('button', { name: /^Keluarkan/ })).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Jadikan owner' })).toHaveLength(1);
    expect(screen.getAllByRole('button', { name: 'Jadikan member' })).toHaveLength(2);
    expect(screen.getAllByRole('button', { name: 'Keluar' })).toHaveLength(1);
    expect(screen.getByText('(Anda)')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Jadikan owner' }));
    expect(p.onrequestrole).toHaveBeenCalledWith(expect.objectContaining({ name: 'Budi' }), 'owner');
    await userEvent.click(screen.getByRole('button', { name: 'Keluarkan Budi' }));
    expect(p.onrequestremove).toHaveBeenCalled();
  });
  it('T03-AC7: admin without own row manages every row', () => {
    render(MembersTable, props('admin'));
    expect(screen.getAllByRole('button', { name: /^Keluarkan/ })).toHaveLength(3);
    expect(screen.queryByRole('button', { name: 'Keluar' })).toBeNull();
  });
  it('shows Nonaktif, and unknown user + Tidak ditemukan', () => {
    render(MembersTable, {
      ...props('owner'),
      items: [
        mockMember(2, { name: 'Dina', is_active: false }),
        mockMember(3, { name: null, email: null, found: false, is_active: false }),
      ],
    });
    expect(screen.getByText('Nonaktif')).toBeInTheDocument();
    expect(screen.getByText('User tidak dikenal')).toBeInTheDocument();
    expect(screen.getByText('Tidak ditemukan')).toBeInTheDocument();
  });
  it('busy row shows loading and disables others', () => {
    render(MembersTable, { ...props('owner'), busyUserId: memberId(2) });
    expect(screen.getByRole('button', { name: /Jadikan owner/ })).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByRole('button', { name: 'Keluar' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Keluarkan Citra' })).toBeDisabled();
  });
});
