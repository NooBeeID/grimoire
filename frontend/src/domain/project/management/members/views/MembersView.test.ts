import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import { memberId, mockCandidate, mockMember } from '../mocks/members';

vi.mock('../services/members.service', async (orig) => ({
  ...(await orig<typeof import('../services/members.service')>()),
  listMembers: vi.fn(),
  searchCandidates: vi.fn(),
  addMember: vi.fn(),
  removeMember: vi.fn(),
  changeMemberRole: vi.fn(),
}));
vi.mock('../services/invitations.service', async (orig) => ({
  ...(await orig<typeof import('../services/invitations.service')>()),
  listInvitations: vi.fn(async () => []),
}));
import { addMember, changeMemberRole, listMembers, removeMember, searchCandidates } from '../services/members.service';
import View from './MembersView.svelte';

const mList = vi.mocked(listMembers);
const mSearch = vi.mocked(searchCandidates);
const mAdd = vi.mocked(addMember);
const mRemove = vi.mocked(removeMember);
const mRole = vi.mocked(changeMemberRole);

const ME = memberId(1);
const owner = (n: number, name: string) => mockMember(n, { name, role: 'owner' });
const member = (n: number, name: string) => mockMember(n, { name });
const err = (status: number, code: string, details: unknown = null) => new ApiError({ status, code, message: 'pesan server', details });
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1',
  accessLevel: 'owner' as const,
  currentUserId: ME,
  onleft: vi.fn(),
  onaccesschanged: vi.fn(),
  onprojectgone: vi.fn(),
  ...over,
});
const toastMessages = () => get(toasts).map((x) => x.message);

beforeEach(() => {
  setLocale('id');
  [mList, mSearch, mAdd, mRemove, mRole].forEach((m) => m.mockReset());
});
afterEach(() => {
  vi.useRealTimers();
  toast.clear();
});

describe('MembersView: list', () => {
  it('loads and shows rows', async () => {
    mList.mockResolvedValue([owner(1, 'Ana'), member(2, 'Budi')]);
    render(View, props());
    expect(await screen.findByText('Budi')).toBeInTheDocument();
    expect(mList).toHaveBeenCalledTimes(1);
    expect(mList.mock.calls[0]![0]).toBe('p1');
  });
  it('500 shows ErrorState without rows', async () => {
    mList.mockRejectedValue(err(500, 'INTERNAL'));
    render(View, props());
    expect(await screen.findByText('pesan server')).toBeInTheDocument();
    expect(screen.queryByRole('table')).toBeNull();
  });
  it('404 calls onprojectgone', async () => {
    mList.mockRejectedValue(err(404, 'NOT_FOUND'));
    const p = props();
    render(View, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
  it('member does not see add button', async () => {
    mList.mockResolvedValue([owner(2, 'Ana'), member(1, 'Me')]);
    render(View, props({ accessLevel: 'member' }));
    await screen.findByText('Ana');
    expect(screen.queryByRole('button', { name: 'Tambah anggota' })).toBeNull();
  });
});

describe('MembersView: add', () => {
  async function openAdd() {
    mList.mockResolvedValue([owner(1, 'Ana'), member(2, 'Budi')]);
    const p = props();
    render(View, p);
    await screen.findByText('Budi');
    await userEvent.click(screen.getByRole('button', { name: 'Tambah anggota' }));
    return p;
  }

  it('debounces to a single request, filters existing members, add disabled until selection', async () => {
    mSearch.mockResolvedValue([mockCandidate(1), { ...mockCandidate(2), id: memberId(2), name: 'Budi' }]);
    await openAdd();
    const dlg = screen.getByRole('dialog', { hidden: false });
    const input = within(dlg).getByRole('combobox');
    const submit = within(dlg).getByRole('button', { name: 'Tambah' });
    expect(submit).toBeDisabled();
    await userEvent.type(input, 'bud');
    await vi.waitFor(() => expect(mSearch).toHaveBeenCalledTimes(1));
    expect(mSearch.mock.calls[0]![1]).toBe('bud');
    const opts = await within(dlg).findAllByRole('option');
    expect(opts).toHaveLength(1);
    expect(opts[0]).toHaveTextContent('Budi 1');
    await userEvent.keyboard('{ArrowDown}{Enter}');
    expect(within(dlg).getByTestId('selected-candidate')).toHaveTextContent('Budi 1');
    expect(within(dlg).getByRole('button', { name: 'Tambah' })).toBeEnabled();
  });

  it('typing again aborts the previous request', async () => {
    mSearch.mockImplementation(() => new Promise(() => {}));
    await openAdd();
    const input = screen.getByRole('combobox');
    await userEvent.type(input, 'b');
    await vi.waitFor(() => expect(mSearch).toHaveBeenCalledTimes(1));
    const sig = mSearch.mock.calls[0]![2];
    await userEvent.type(input, 'u');
    await vi.waitFor(() => expect(mSearch).toHaveBeenCalledTimes(2));
    expect(sig?.aborted).toBe(true);
  });

  async function pick() {
    mSearch.mockResolvedValue([mockCandidate(1)]);
    await userEvent.type(screen.getByRole('combobox'), 'bud');
    await screen.findByRole('option');
    await userEvent.keyboard('{ArrowDown}{Enter}');
  }

  it('success adds the row, closes the dialog and toasts', async () => {
    await openAdd();
    await pick();
    mAdd.mockResolvedValue(mockMember(101, { name: 'Budi 1', email: 'budi1@tim.id' }));
    await userEvent.click(screen.getByRole('button', { name: 'Tambah' }));
    expect(await within(screen.getByRole('table')).findByText('Budi 1')).toBeInTheDocument();
    expect(mAdd).toHaveBeenCalledWith('p1', mockCandidate(1).id, 'member');
    expect(toastMessages()).toContain('Budi 1 ditambahkan');
  });

  it.each([
    ['USER_INACTIVE', 400, 'User nonaktif tidak bisa ditambahkan'],
    ['USER_NOT_DEVELOPER', 400, 'Hanya developer yang bisa ditambahkan. Super Admin tidak perlu menjadi anggota.'],
    ['USER_NOT_FOUND', 400, 'User tidak ditemukan'],
  ])('T03-AC4: %s shows the i18n message in the dialog', async (reason, status, text) => {
    await openAdd();
    await pick();
    mAdd.mockRejectedValue(err(status, 'INVALID_ARGUMENT', { reason }));
    await userEvent.click(screen.getByRole('button', { name: 'Tambah' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(text);
  });

  it('409 ALREADY_MEMBER shows message and reloads the list', async () => {
    await openAdd();
    await pick();
    mAdd.mockRejectedValue(err(409, 'CONFLICT', { reason: 'ALREADY_MEMBER' }));
    await userEvent.click(screen.getByRole('button', { name: 'Tambah' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('User sudah menjadi anggota');
    await vi.waitFor(() => expect(mList).toHaveBeenCalledTimes(2));
  });

  it('403 closes the dialog, toasts and calls onaccesschanged', async () => {
    const p = await openAdd();
    await pick();
    mAdd.mockRejectedValue(err(403, 'FORBIDDEN'));
    await userEvent.click(screen.getByRole('button', { name: 'Tambah' }));
    await vi.waitFor(() => expect(p.onaccesschanged).toHaveBeenCalled());
    expect(toastMessages()).toContain('Anda tidak memiliki akses');
  });
});

describe('MembersView: role, remove, leave', () => {
  it('T03-AC3: last owner leaving gets LAST_OWNER in dialog, onleft not called, row stays', async () => {
    mList.mockResolvedValue([owner(1, 'Ana'), member(2, 'Budi')]);
    mRemove.mockRejectedValue(err(409, 'CONFLICT', { reason: 'LAST_OWNER' }));
    const p = props();
    render(View, p);
    await screen.findByText('Budi');
    await userEvent.click(screen.getByRole('button', { name: 'Keluar' }));
    const dlg = screen.getByRole('dialog');
    await userEvent.click(within(dlg).getByRole('button', { name: 'Keluar' }));
    expect(await within(dlg).findByRole('alert')).toHaveTextContent('Owner terakhir tidak bisa keluar atau diturunkan');
    expect(p.onleft).not.toHaveBeenCalled();
    expect(screen.getByText('Ana')).toBeInTheDocument();
  });

  it('leave success calls onleft', async () => {
    mList.mockResolvedValue([owner(1, 'Ana'), owner(2, 'Budi')]);
    mRemove.mockResolvedValue(undefined);
    const p = props();
    render(View, p);
    await screen.findByText('Budi');
    await userEvent.click(screen.getByRole('button', { name: 'Keluar' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Keluar' }));
    await vi.waitFor(() => expect(p.onleft).toHaveBeenCalled());
    expect(toastMessages()).toContain('Anda telah keluar dari project');
  });

  it('promote goes without dialog and reorders', async () => {
    mList.mockResolvedValue([owner(1, 'Ana'), member(2, 'Budi')]);
    mRole.mockResolvedValue(mockMember(2, { name: 'Budi', role: 'owner' }));
    render(View, props());
    await screen.findByText('Budi');
    await userEvent.click(screen.getByRole('button', { name: 'Jadikan owner' }));
    await vi.waitFor(() => expect(mRole).toHaveBeenCalledWith('p1', memberId(2), 'owner'));
    expect(toastMessages()).toContain('Peran diperbarui');
    expect(screen.queryByRole('button', { name: 'Jadikan owner' })).toBeNull();
  });

  it('demoting myself succeeds -> onaccesschanged', async () => {
    mList.mockResolvedValue([owner(1, 'Ana'), owner(2, 'Budi')]);
    mRole.mockResolvedValue(mockMember(1, { name: 'Ana', role: 'member' }));
    const p = props();
    render(View, p);
    await screen.findByText('Budi');
    const rowSelf = screen.getAllByRole('row').find((r) => within(r).queryByText('(Anda)'))!;
    await userEvent.click(within(rowSelf).getByRole('button', { name: 'Jadikan member' }));
    const dlg = screen.getByRole('dialog');
    expect(within(dlg).getByText(/Anda akan kehilangan akses kelola anggota/)).toBeInTheDocument();
    await userEvent.click(within(dlg).getByRole('button', { name: 'Jadikan member' }));
    await vi.waitFor(() => expect(p.onaccesschanged).toHaveBeenCalled());
  });

  it('demoting the last owner shows LAST_OWNER in the dialog', async () => {
    mList.mockResolvedValue([owner(1, 'Ana')]);
    mRole.mockRejectedValue(err(409, 'CONFLICT', { reason: 'LAST_OWNER' }));
    render(View, props());
    await screen.findByText('Ana');
    await userEvent.click(screen.getByRole('button', { name: 'Jadikan member' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Jadikan member' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Owner terakhir');
  });

  it('remove 404 and list still 200 -> refresh + gone toast', async () => {
    mList.mockResolvedValueOnce([owner(1, 'Ana'), member(2, 'Budi')]).mockResolvedValueOnce([owner(1, 'Ana')]);
    mRemove.mockRejectedValue(err(404, 'NOT_FOUND'));
    const p = props();
    render(View, p);
    await screen.findByText('Budi');
    await userEvent.click(screen.getByRole('button', { name: /^Keluarkan/ }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Keluarkan' }));
    await vi.waitFor(() => expect(toastMessages()).toContain('Anggota sudah tidak ada; daftar diperbarui'));
    expect(mList).toHaveBeenCalledTimes(2);
    expect(screen.queryByText('Budi')).toBeNull();
    expect(p.onprojectgone).not.toHaveBeenCalled();
  });

  it('remove 404 and list 404 -> onprojectgone', async () => {
    mList.mockResolvedValueOnce([owner(1, 'Ana'), member(2, 'Budi')]).mockRejectedValueOnce(err(404, 'NOT_FOUND'));
    mRemove.mockRejectedValue(err(404, 'NOT_FOUND'));
    const p = props();
    render(View, p);
    await screen.findByText('Budi');
    await userEvent.click(screen.getByRole('button', { name: /^Keluarkan/ }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Keluarkan' }));
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });

  it('403 on an action toasts and calls onaccesschanged', async () => {
    mList.mockResolvedValue([owner(1, 'Ana'), member(2, 'Budi')]);
    mRole.mockRejectedValue(err(403, 'FORBIDDEN'));
    const p = props();
    render(View, p);
    await screen.findByText('Budi');
    await userEvent.click(screen.getByRole('button', { name: 'Jadikan owner' }));
    await vi.waitFor(() => expect(p.onaccesschanged).toHaveBeenCalled());
    expect(toastMessages()).toContain('Anda tidak memiliki akses');
  });
});
