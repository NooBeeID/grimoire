import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockInvitations, mockNow, mockPendingMembers } from '../mocks/invitations';
import { mockMember } from '../mocks/members';
import type { InvitationsState } from '../stores/invitationsList.store';
import InvitationStatusPill from './InvitationStatusPill.svelte';
import InvitationsSection from './InvitationsSection.svelte';
import InvitationsTable from './InvitationsTable.svelte';
import InviteByEmailForm from './InviteByEmailForm.svelte';
import MembersTable from './MembersTable.svelte';

const cbs = () => ({ onrequestresend: vi.fn(), onrequestrevoke: vi.fn() });
const ready = (items = mockInvitations): InvitationsState => ({ status: 'ready', items, error: null, refreshing: false });

describe('InviteByEmailForm', () => {
  const props = (over: Record<string, unknown> = {}) => ({
    email: 'baru@tim.id',
    emailEnabled: false,
    onsubmit: vi.fn(),
    onback: vi.fn(),
    oncancel: vi.fn(),
    onsearchinstead: vi.fn(),
    ...over,
  });
  it('email terkunci, Ganti memanggil onback, peran default member, submit memanggil onsubmit', async () => {
    const p = props();
    render(InviteByEmailForm, p);
    expect(screen.getByTestId('invite-email')).toHaveTextContent('baru@tim.id');
    expect(screen.getByRole('radio', { name: 'Member' })).toBeChecked();
    expect(screen.getByTestId('email-delivery-note')).toHaveTextContent('Email belum diatur');
    await userEvent.click(screen.getByRole('button', { name: 'Ganti' }));
    expect(p.onback).toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Kirim undangan' }));
    expect(p.onsubmit).toHaveBeenCalled();
  });
  it('error field email tampil di bawah baris email', () => {
    render(InviteByEmailForm, props({ error: { kind: 'fields', fields: { email: 'format salah' } } }));
    expect(screen.getByText('format salah')).toBeInTheDocument();
  });
  it('alreadyRegistered.registered menampilkan Cari developer', async () => {
    const p = props({ error: { kind: 'alreadyRegistered', reason: 'registered' } });
    render(InviteByEmailForm, p);
    await userEvent.click(screen.getByRole('button', { name: 'Cari developer' }));
    expect(p.onsearchinstead).toHaveBeenCalled();
  });
  it('rateLimited menampilkan menit; submitting menonaktifkan tombol', () => {
    render(InviteByEmailForm, props({ submitting: true, error: { kind: 'rateLimited', scope: 'invite', minutes: 31 } }));
    expect(screen.getByRole('alert')).toHaveTextContent('Coba lagi dalam 31 menit');
    expect(screen.getByRole('button', { name: /Kirim undangan/ })).toBeDisabled();
  });
  it('generic menampilkan pesan dan ID jejak', () => {
    render(InviteByEmailForm, props({ error: { kind: 'generic', message: 'gagal', traceId: 'tr9' } }));
    expect(screen.getByRole('alert')).toHaveTextContent('gagal');
    expect(screen.getByRole('alert')).toHaveTextContent('tr9');
  });
});

describe('InvitationStatusPill', () => {
  it.each([
    [{ expired: true, delivery_status: 'sent' as const }, 'Kedaluwarsa', 'bg-pill-warning-bg'],
    [{ expired: false, delivery_status: 'failed' as const }, 'Email gagal terkirim', 'bg-pill-negative-bg'],
    [{ expired: false, delivery_status: 'not_configured' as const }, 'Kirim manual', 'bg-pill-neutral-bg'],
    [{ expired: false, delivery_status: 'pending' as const }, 'Belum terkirim', 'bg-pill-neutral-bg'],
    [{ expired: false, delivery_status: 'sent' as const }, 'Menunggu login', 'bg-pill-neutral-bg'],
  ])('%j', (invitation, text, cls) => {
    render(InvitationStatusPill, { invitation });
    expect(screen.getByText(text)).toHaveClass(cls);
  });
});

describe('InvitationsTable', () => {
  it('kolom Diundang oleh ada bila terisi; sent_at null -> dash; resend_count tampil', () => {
    render(InvitationsTable, { items: mockInvitations, ...cbs() });
    expect(screen.getByRole('columnheader', { name: 'Diundang oleh' })).toBeInTheDocument();
    expect(screen.getByText('dikirim ulang 1x')).toBeInTheDocument();
    expect(screen.getByText('–')).toBeInTheDocument();
  });
  it('tanpa pengundang, kolom dibuang', () => {
    render(InvitationsTable, { items: mockInvitations.map((i) => ({ ...i, invited_by_label: null })), ...cbs() });
    expect(screen.queryByRole('columnheader', { name: 'Diundang oleh' })).toBeNull();
  });
  it('aksi memanggil callback dengan item dan aria-label menyebut email', async () => {
    const c = cbs();
    render(InvitationsTable, { items: mockInvitations, ...c });
    await userEvent.click(screen.getByRole('button', { name: 'Kirim ulang undangan ke nadia.putri@tim.id' }));
    expect(c.onrequestresend).toHaveBeenCalledWith(mockInvitations[1]);
    await userEvent.click(screen.getByRole('button', { name: 'Batalkan undangan yoga.saputra@tim.id' }));
    expect(c.onrequestrevoke).toHaveBeenCalledWith(mockInvitations[2]);
  });
  it('baris sibuk menonaktifkan aksi baris lain', () => {
    render(InvitationsTable, { items: mockInvitations, busyUserId: mockInvitations[0]!.user_id, ...cbs() });
    expect(screen.getByRole('button', { name: 'Batalkan undangan nadia.putri@tim.id' })).toBeDisabled();
  });
});

describe('InvitationsSection', () => {
  it('ready kosong dan muat pertama tidak dirender', () => {
    const { container, unmount } = render(InvitationsSection, { state: ready([]), onretry: vi.fn(), ...cbs() });
    expect(container.querySelector('section')).toBeNull();
    unmount();
    const second = render(InvitationsSection, { state: { status: 'loading', items: [], error: null, refreshing: false }, onretry: vi.fn(), ...cbs() });
    expect(second.container.querySelector('section')).toBeNull();
  });
  it('error menampilkan ErrorState dengan Coba lagi', async () => {
    const onretry = vi.fn();
    const error = new ApiError({ status: 500, code: 'INTERNAL', message: 'pesan server' });
    render(InvitationsSection, { state: { status: 'error', items: [], error, refreshing: false }, onretry, ...cbs() });
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalled();
  });
  it('jumlah header = items.length', () => {
    render(InvitationsSection, { state: ready(), onretry: vi.fn(), ...cbs() });
    expect(within(screen.getByRole('heading', { level: 3 })).getByText('3')).toBeInTheDocument();
  });
});

describe('MembersTable: penanda undangan', () => {
  const common = { currentUserId: 'x', accessLevel: 'owner' as const, onrequestrole: vi.fn(), onrequestremove: vi.fn(), onrequestleave: vi.fn() };
  it('kolom Status muncul karena invitation_pending dan pil sesuai masa berlaku', () => {
    render(MembersTable, { items: mockPendingMembers, now: mockNow, ...common });
    expect(screen.getByRole('columnheader', { name: 'Status' })).toBeInTheDocument();
    expect(screen.getAllByText('Undangan menunggu')).toHaveLength(2);
    expect(screen.getByText('Undangan kedaluwarsa')).toHaveClass('bg-pill-warning-bg');
  });
  it('!found tetap menang atas penanda undangan', () => {
    render(MembersTable, { items: [mockMember(1, { found: false, invitation_pending: true, invitation_expires_at: '2026-10-28T00:00:00Z' })], now: mockNow, ...common });
    expect(screen.queryByText('Undangan menunggu')).toBeNull();
  });
});
