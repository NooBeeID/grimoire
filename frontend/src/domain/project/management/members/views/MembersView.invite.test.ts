import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import {
  BAGAS_ID,
  mockInvitations,
  mockInviteFallback,
  mockInviteSent,
  mockPendingMembers,
  mockResendFallback,
  mockResendSent,
  OWNER_ID,
} from '../mocks/invitations';
import { mockMember } from '../mocks/members';

vi.mock('../services/members.service', async (orig) => ({
  ...(await orig<typeof import('../services/members.service')>()),
  listMembers: vi.fn(),
  searchCandidates: vi.fn(),
}));
vi.mock('../services/invitations.service', async (orig) => ({
  ...(await orig<typeof import('../services/invitations.service')>()),
  listInvitations: vi.fn(),
  inviteToProject: vi.fn(),
  resendProjectInvitation: vi.fn(),
  revokeInvitation: vi.fn(),
}));
import { listMembers, searchCandidates } from '../services/members.service';
import { inviteToProject, listInvitations, resendProjectInvitation, revokeInvitation } from '../services/invitations.service';
import View from './MembersView.svelte';

const mList = vi.mocked(listMembers);
const mSearch = vi.mocked(searchCandidates);
const mInvList = vi.mocked(listInvitations);
const mInvite = vi.mocked(inviteToProject);
const mResend = vi.mocked(resendProjectInvitation);
const mRevoke = vi.mocked(revokeInvitation);

const owner = mockMember(1, { user_id: OWNER_ID, name: 'Raka Pratama', email: 'raka@tim.id', role: 'owner' });
const err = (status: number, code: string, details: unknown = null) => new ApiError({ status, code, message: 'pesan server', details });
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1',
  accessLevel: 'owner' as const,
  currentUserId: OWNER_ID,
  emailEnabled: false as boolean | null,
  onleft: vi.fn(),
  onaccesschanged: vi.fn(),
  onprojectgone: vi.fn(),
  ...over,
});
const toastMessages = () => get(toasts).map((x) => x.message);

beforeEach(() => {
  setLocale('id');
  [mList, mSearch, mInvList, mInvite, mResend, mRevoke].forEach((m) => m.mockReset());
  mList.mockResolvedValue([owner, ...mockPendingMembers]);
  mInvList.mockResolvedValue(mockInvitations);
  mSearch.mockResolvedValue([]);
});
afterEach(() => toast.clear());

describe('MembersView: daftar undangan', () => {
  it('owner memuat undangan dan section tampil di atas tabel anggota', async () => {
    render(View, props());
    const section = await screen.findByTestId('invitations-section');
    expect(within(section).getAllByRole('row')).toHaveLength(4);
    expect(mInvList).toHaveBeenCalledTimes(1);
    const tables = screen.getAllByRole('table');
    expect(tables).toHaveLength(2);
    expect(section.compareDocumentPosition(tables[1]!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
  it('member biasa tidak memanggil endpoint undangan', async () => {
    render(View, props({ accessLevel: 'member' }));
    await screen.findByText('Raka Pratama');
    expect(mInvList).not.toHaveBeenCalled();
    expect(screen.queryByTestId('invitations-section')).toBeNull();
  });
  it('500 menampilkan error di section, tabel anggota tetap', async () => {
    mInvList.mockRejectedValue(err(500, 'INTERNAL'));
    render(View, props());
    const section = await screen.findByTestId('invitations-section');
    expect(within(section).getByRole('button', { name: 'Coba lagi' })).toBeInTheDocument();
    expect(screen.getByText('Raka Pratama')).toBeInTheDocument();
  });
  it('404 memanggil onprojectgone; 403 menyembunyikan section dan onaccesschanged', async () => {
    mInvList.mockRejectedValue(err(404, 'NOT_FOUND'));
    const p = props();
    const { unmount } = render(View, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
    unmount();
    mInvList.mockRejectedValue(err(403, 'FORBIDDEN'));
    const p2 = props();
    render(View, p2);
    await vi.waitFor(() => expect(p2.onaccesschanged).toHaveBeenCalled());
    expect(screen.queryByTestId('invitations-section')).toBeNull();
  });
});

describe('MembersView: undang lewat email', () => {
  async function toInviteMode() {
    const p = props();
    render(View, p);
    await screen.findByTestId('invitations-section');
    await userEvent.click(screen.getByRole('button', { name: 'Tambah anggota' }));
    await userEvent.type(screen.getByRole('combobox'), 'baru@tim.id');
    await userEvent.click(await screen.findByTestId('invite-option'));
    return p;
  }

  it('memilih opsi mengubah judul dialog dan mengirim body lengkap', async () => {
    mInvite.mockResolvedValue(mockInviteSent);
    await toInviteMode();
    const dlg = screen.getByRole('dialog');
    expect(within(dlg).getByRole('heading', { name: 'Undang lewat email' })).toBeInTheDocument();
    expect(within(dlg).getByTestId('invite-email')).toHaveTextContent('baru@tim.id');
    await userEvent.type(within(dlg).getByLabelText('Nama (opsional)'), 'Nadia Putri');
    await userEvent.click(within(dlg).getByRole('radio', { name: 'Owner' }));
    await userEvent.click(within(dlg).getByRole('button', { name: 'Kirim undangan' }));
    expect(mInvite).toHaveBeenCalledWith('p1', { email: 'baru@tim.id', role: 'owner', name: 'Nadia Putri' });
  });
  it('terkirim: toast sukses, undangan diambil ulang, tanpa dialog kredensial', async () => {
    mInvite.mockResolvedValue(mockInviteSent);
    await toInviteMode();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    await vi.waitFor(() => expect(toastMessages()).toContain('Undangan dikirim ke nadia.putri@tim.id'));
    expect(mInvList).toHaveBeenCalledTimes(2);
    expect(screen.queryByTestId('temporary-password')).toBeNull();
  });
  it('fallback: dialog kredensial memuat password; tutup menghapusnya dari DOM', async () => {
    mInvite.mockResolvedValue(mockInviteFallback);
    await toInviteMode();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    const pw = await screen.findByTestId('temporary-password');
    expect(pw).toHaveValue('q7XkR2mVtz9HbNcP');
    await userEvent.click(screen.getByRole('button', { name: 'Saya sudah menyalin' }));
    await vi.waitFor(() => expect(document.body.innerHTML).not.toContain('q7XkR2mVtz9HbNcP'));
  });
  it('tidak terkirim tanpa credentials: toast error, anggota tercatat', async () => {
    mInvite.mockResolvedValue({ ...mockInviteSent, email_sent: false, email_status: 'failed' as const });
    await toInviteMode();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    await vi.waitFor(() => expect(toastMessages().some((m) => m.includes('gagal dikirim'))).toBe(true));
  });
  it('409 ALREADY_REGISTERED -> pesan dan Cari developer kembali ke mode cari dengan email terisi', async () => {
    mInvite.mockRejectedValue(err(409, 'ALREADY_REGISTERED', { reason: 'registered' }));
    await toInviteMode();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    expect(await screen.findByText(/sudah punya akun/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Cari developer' }));
    expect(screen.getByRole('combobox')).toHaveValue('baru@tim.id');
  });
  it('409 ALREADY_MEMBER -> pesan inline dan muat ulang anggota', async () => {
    mInvite.mockRejectedValue(err(409, 'CONFLICT', { reason: 'ALREADY_MEMBER' }));
    await toInviteMode();
    const before = mList.mock.calls.length;
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    expect(await screen.findByText('User sudah menjadi anggota')).toBeInTheDocument();
    expect(mList.mock.calls.length).toBeGreaterThan(before);
  });
  it('429 menampilkan menit', async () => {
    mInvite.mockRejectedValue(err(429, 'RATE_LIMITED', { retry_after_seconds: 1840, scope: 'invite' }));
    await toInviteMode();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    expect(await screen.findByText(/Coba lagi dalam 31 menit/)).toBeInTheDocument();
  });
  it('403 menutup dialog, toast, onaccesschanged', async () => {
    mInvite.mockRejectedValue(err(403, 'FORBIDDEN'));
    const p = await toInviteMode();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    await vi.waitFor(() => expect(p.onaccesschanged).toHaveBeenCalled());
    expect(toastMessages()).toContain('Anda tidak memiliki akses');
  });
  it('catatan pengiriman mengikuti emailEnabled', async () => {
    const p = props({ emailEnabled: null });
    render(View, p);
    await screen.findByTestId('invitations-section');
    await userEvent.click(screen.getByRole('button', { name: 'Tambah anggota' }));
    await userEvent.type(screen.getByRole('combobox'), 'baru@tim.id');
    await userEvent.click(await screen.findByTestId('invite-option'));
    expect(screen.getByTestId('email-delivery-note')).toHaveTextContent('Bila email gagal terkirim');
  });
});

describe('MembersView: kirim ulang dan batalkan', () => {
  const nadia = () => screen.getByRole('button', { name: 'Kirim ulang undangan ke nadia.putri@tim.id' });
  it('kirim ulang terkirim: toast dan muat ulang undangan', async () => {
    mResend.mockResolvedValue(mockResendSent);
    render(View, props());
    await screen.findByTestId('invitations-section');
    await userEvent.click(nadia());
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Kirim ulang' }));
    await vi.waitFor(() => expect(toastMessages()).toContain('Undangan dikirim ulang ke nadia.putri@tim.id'));
    expect(mInvList).toHaveBeenCalledTimes(2);
  });
  it('kirim ulang fallback membuka dialog kredensial', async () => {
    mResend.mockResolvedValue(mockResendFallback);
    render(View, props());
    await screen.findByTestId('invitations-section');
    await userEvent.click(screen.getByRole('button', { name: 'Kirim ulang undangan ke bagas@tim.id' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Kirim ulang' }));
    expect(await screen.findByTestId('temporary-password')).toHaveValue('Zp4wNe8TqYv3KdRh');
  });
  it('429 resend -> pesan inline di konfirmasi', async () => {
    mResend.mockRejectedValue(err(429, 'RATE_LIMITED', { retry_after_seconds: 1840, scope: 'resend' }));
    render(View, props());
    await screen.findByTestId('invitations-section');
    await userEvent.click(nadia());
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Kirim ulang' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Coba lagi dalam 31 menit');
  });
  it('409 INVITATION_NOT_PENDING pada resend -> toast dan refresh kedua daftar', async () => {
    mResend.mockRejectedValue(err(409, 'CONFLICT', { reason: 'INVITATION_NOT_PENDING' }));
    render(View, props());
    await screen.findByTestId('invitations-section');
    const before = mList.mock.calls.length;
    await userEvent.click(nadia());
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Kirim ulang' }));
    await vi.waitFor(() => expect(toastMessages()).toContain('Undangan sudah diterima atau tidak ada lagi.'));
    expect(mInvList).toHaveBeenCalledTimes(2);
    expect(mList.mock.calls.length).toBeGreaterThan(before);
  });
  it('batalkan: pesan konfirmasi, DELETE, baris hilang dari kedua daftar tanpa GET ulang', async () => {
    mRevoke.mockResolvedValue(undefined);
    render(View, props());
    await screen.findByTestId('invitations-section');
    await userEvent.click(screen.getByRole('button', { name: 'Batalkan undangan bagas@tim.id' }));
    const dlg = screen.getByRole('dialog');
    expect(dlg).toHaveTextContent('Akunnya tetap ada');
    await userEvent.click(within(dlg).getByRole('button', { name: 'Batalkan' }));
    await vi.waitFor(() => expect(mRevoke).toHaveBeenCalledWith('p1', BAGAS_ID));
    await vi.waitFor(() => expect(screen.queryAllByText('bagas@tim.id')).toHaveLength(0));
    expect(toastMessages()).toContain('Undangan dibatalkan');
    expect(mInvList).toHaveBeenCalledTimes(1);
  });
  it('batalkan: INVITATION_NOT_PENDING -> pesan inline; LAST_OWNER -> pesan project', async () => {
    mRevoke.mockRejectedValueOnce(err(409, 'CONFLICT', { reason: 'INVITATION_NOT_PENDING' }));
    render(View, props());
    await screen.findByTestId('invitations-section');
    await userEvent.click(screen.getByRole('button', { name: 'Batalkan undangan bagas@tim.id' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Batalkan' }));
    expect(await screen.findByText(/sudah diterima. Gunakan Keluarkan/)).toBeInTheDocument();
  });
});
