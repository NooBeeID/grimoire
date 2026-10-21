import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { setLocale } from '../../../../shared/stores/i18n';
import InviteDeveloperDialog from './components/InviteDeveloperDialog.svelte';
import ResendInvitationDialog from './components/ResendInvitationDialog.svelte';
import UserRowActions from './components/UserRowActions.svelte';
import UsersTable from './components/UsersTable.svelte';
import { mockDimas, mockRaka, mockRani, mockUserInviteFallback, mockUserResendFallback, mockYoga } from './mocks/invitations';
import { inviteUser, resendUserInvitation } from './services/users.service';
import { InviteUserResponseSchema, toUserFormError, UserSchema } from './types';

beforeEach(() => setLocale('id'));

describe('types: undangan', () => {
  it('UserSchema menerima invitation null dan objek lengkap; InviteUserResponse dengan dan tanpa credentials', () => {
    expect(UserSchema.safeParse(mockRaka).success).toBe(true);
    expect(UserSchema.safeParse(mockRani).success).toBe(true);
    expect(UserSchema.safeParse({ ...mockRaka, invitation: undefined }).success).toBe(false);
    expect(InviteUserResponseSchema.safeParse(mockUserInviteFallback).success).toBe(true);
    const { credentials: _c, ...noCreds } = mockUserInviteFallback;
    expect(InviteUserResponseSchema.safeParse({ ...noCreds, email_sent: true, email_status: 'sent' }).success).toBe(true);
  });
  const err = (status: number, code: string, details: unknown = null) => new ApiError({ status, code, message: 'm', details });
  it('toUserFormError: ALREADY_REGISTERED per reason', () => {
    const r = toUserFormError(err(409, 'ALREADY_REGISTERED', { reason: 'registered' }));
    expect(r).toMatchObject({ kind: 'form', reason: 'registered' });
    expect(r.kind === 'form' && r.fields.email).toContain('sudah punya akun');
    expect(toUserFormError(err(409, 'ALREADY_REGISTERED', { reason: 'inactive' }))).toMatchObject({ reason: 'inactive' });
    expect(toUserFormError(err(409, 'ALREADY_REGISTERED', { reason: 'not_developer' }))).toMatchObject({ reason: 'not_developer' });
  });
  it('toUserFormError: INVITATION_NOT_PENDING, 429, dan 409 lama', () => {
    expect(toUserFormError(err(409, 'CONFLICT', { reason: 'INVITATION_NOT_PENDING' }))).toMatchObject({ message: 'Undangan sudah diterima atau tidak ada lagi.' });
    expect(toUserFormError(err(429, 'RATE_LIMITED', { retry_after_seconds: 1840, scope: 'invite' }))).toMatchObject({
      message: 'Batas undangan per jam tercapai. Coba lagi dalam 31 menit.',
    });
    expect(toUserFormError(err(429, 'RATE_LIMITED', { retry_after_seconds: 1840, scope: 'resend' }))).toMatchObject({
      message: 'Undangan ini terlalu sering dikirim ulang. Coba lagi dalam 31 menit.',
    });
    expect(toUserFormError(err(409, 'CONFLICT'))).toMatchObject({ fields: { email: 'Email sudah terdaftar' } });
  });
});

describe('users.service: undangan', () => {
  const fetchMock = vi.fn();
  const json = (b: unknown, s = 200) => new Response(JSON.stringify(b), { status: s });
  beforeEach(() => {
    vi.stubGlobal('fetch', fetchMock);
    fetchMock.mockReset();
  });
  afterEach(() => vi.unstubAllGlobals());
  const call = () => fetchMock.mock.calls[0]! as [string, RequestInit];

  it('inviteUser: tanpa name kosong, email dinormalisasi, dengan name bila diisi', async () => {
    fetchMock.mockImplementation(async () => json(mockUserInviteFallback, 201));
    await inviteUser({ email: ' Rani@Tim.ID ', name: '' });
    expect(call()[0]).toBe('/api/users/invite');
    expect(JSON.parse(call()[1].body as string)).toEqual({ email: 'rani@tim.id' });
    fetchMock.mockClear();
    await inviteUser({ email: 'rani@tim.id', name: 'Rani A' });
    expect(JSON.parse(call()[1].body as string)).toEqual({ email: 'rani@tim.id', name: 'Rani A' });
  });
  it('resendUserInvitation: POST tanpa body ke path ber-encode', async () => {
    fetchMock.mockResolvedValue(json(mockUserResendFallback));
    const r = await resendUserInvitation('a/b');
    expect(call()[0]).toBe('/api/users/a%2Fb/resend-invitation');
    expect(call()[1].body).toBeUndefined();
    expect(r.credentials?.temporary_password).toBe('Zp4wNe8TqYv3KdRh');
  });
});

describe('InviteDeveloperDialog', () => {
  const props = (over: Record<string, unknown> = {}) => ({ open: true, emailEnabled: true, onsubmit: vi.fn(), onshowinlist: vi.fn(), onclose: vi.fn(), ...over });
  it('validasi lokal: kosong dan bentuk salah tidak memanggil onsubmit', async () => {
    const p = props();
    render(InviteDeveloperDialog, p);
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    expect(screen.getByText('Wajib diisi')).toBeInTheDocument();
    await userEvent.type(screen.getByLabelText(/^Email/), 'bukan-email');
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    expect(screen.getByText('Format email tidak valid')).toBeInTheDocument();
    expect(p.onsubmit).not.toHaveBeenCalled();
  });
  it('submit valid memanggil onsubmit({email, name})', async () => {
    const p = props();
    render(InviteDeveloperDialog, p);
    await userEvent.type(screen.getByLabelText(/^Email/), 'rani@tim.id');
    await userEvent.type(screen.getByLabelText('Nama (opsional)'), 'Rani');
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    expect(p.onsubmit).toHaveBeenCalledWith({ email: 'rani@tim.id', name: 'Rani' });
  });
  it('error registered menampilkan Tampilkan di daftar', async () => {
    const error = toUserFormError(new ApiError({ status: 409, code: 'ALREADY_REGISTERED', message: 'm', details: { reason: 'registered' } }));
    const p = props({ error });
    render(InviteDeveloperDialog, p);
    await userEvent.type(screen.getByLabelText(/^Email/), 'Sari@Tim.id');
    await userEvent.click(screen.getByRole('button', { name: 'Tampilkan di daftar' }));
    expect(p.onshowinlist).toHaveBeenCalledWith('sari@tim.id');
  });
  it.each([
    [true, 'email berisi alamat login'],
    [false, 'Email belum diatur'],
    [null, 'Bila email gagal terkirim'],
  ])('catatan pengiriman %s', (v, text) => {
    render(InviteDeveloperDialog, props({ emailEnabled: v }));
    expect(screen.getByTestId('email-delivery-note')).toHaveTextContent(text);
  });
  it('submitting menonaktifkan tombol', () => {
    render(InviteDeveloperDialog, props({ submitting: true }));
    expect(screen.getByRole('button', { name: /Kirim undangan/ })).toBeDisabled();
  });
});

describe('ResendInvitationDialog', () => {
  it('menyebut email, konfirmasi memanggil onconfirm, error tampil role=alert', async () => {
    const onconfirm = vi.fn();
    const error = { kind: 'form' as const, fields: {}, message: 'Batas habis' };
    render(ResendInvitationDialog, { open: true, user: mockRani, error, onconfirm, onclose: vi.fn() });
    const dlg = screen.getByRole('dialog');
    expect(dlg).toHaveTextContent('rani@tim.id');
    expect(within(dlg).getByRole('alert')).toHaveTextContent('Batas habis');
    await userEvent.click(within(dlg).getByRole('button', { name: 'Kirim ulang' }));
    expect(onconfirm).toHaveBeenCalled();
  });
});

describe('UsersTable: undangan', () => {
  const cbs = { onedit: vi.fn(), onstatus: vi.fn(), onreset: vi.fn(), onresend: vi.fn() };
  it('pil undangan menggantikan Wajib ganti password; warna sesuai status; tanggal berlaku', () => {
    render(UsersTable, { users: [mockRaka, mockRani, mockDimas, mockYoga], ...cbs });
    expect(screen.queryByText('Wajib ganti password')).toBeNull();
    expect(screen.getByText('Undangan menunggu')).toHaveClass('bg-pill-neutral-bg');
    expect(screen.getByText('Email undangan gagal')).toHaveClass('bg-pill-negative-bg');
    expect(screen.getByText('Undangan kedaluwarsa')).toHaveClass('bg-pill-warning-bg');
    expect(screen.getAllByText(/^Berlaku s\.d\./)).toHaveLength(3);
  });
});

describe('UserRowActions: undangan', () => {
  const base = { onedit: vi.fn(), onstatus: vi.fn(), onreset: vi.fn() };
  it('undangan aktif: Kirim ulang undangan menggantikan Reset password', async () => {
    const onresend = vi.fn();
    render(UserRowActions, { user: mockRani, ...base, onresend });
    expect(screen.queryByRole('button', { name: 'Reset password' })).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim ulang undangan' }));
    expect(onresend).toHaveBeenCalledWith(mockRani);
  });
  it('undangan nonaktif: tanpa kirim ulang; akun biasa tetap Reset password', () => {
    const { unmount } = render(UserRowActions, { user: { ...mockRani, is_active: false }, ...base, onresend: vi.fn() });
    expect(screen.queryByRole('button', { name: 'Kirim ulang undangan' })).toBeNull();
    unmount();
    render(UserRowActions, { user: mockRaka, ...base, onresend: vi.fn() });
    expect(screen.getByRole('button', { name: 'Reset password' })).toBeInTheDocument();
  });
});
