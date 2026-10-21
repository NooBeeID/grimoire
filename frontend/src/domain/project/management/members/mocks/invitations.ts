// Data mock sesuai skema Invitation / InviteResponse (api/modules/project/management.yml, kontrak T12).
// "Hari ini" = 2026-10-21 10:30 UTC, masa berlaku 7 hari. Proyek "Payments Gateway", owner Raka Pratama.
import { mockMember } from './members';

export const mockProjectId = '6f2d8d3a-1c1e-4a55-8f40-3d2b6a9e0c11';
export const mockNow = new Date('2026-10-21T10:30:00Z');

export const BAGAS_ID = 'c4a1e9d2-7b3f-4e8a-9c5d-2f6b1a0e8d34';
export const NADIA_ID = '7c1f3c2e-5b7a-4d7e-9a51-1f0b8a3e6d21';
export const YOGA_ID = '3a9d1e47-62c0-4f0b-8e1d-5c7a9b2f4e10';
export const OWNER_ID = '2b6a0f5e-8f3a-4c0a-9d2e-5c1f7a9b3e10';

const BAGAS_EXPIRES = '2026-10-28T09:02:00Z';
const NADIA_EXPIRES = '2026-10-28T08:15:00Z';
const YOGA_EXPIRES = '2026-10-19T03:00:00Z';

export const mockInvitations = [
  {
    user_id: BAGAS_ID,
    email: 'bagas@tim.id',
    name: 'bagas',
    role: 'member' as const,
    invited_by_label: 'raka@tim.id',
    added_at: '2026-10-21T09:02:00Z',
    sent_at: null as string | null,
    expires_at: BAGAS_EXPIRES,
    expired: false,
    delivery_status: 'failed' as const,
    email_failed: true,
    resend_count: 1,
  },
  {
    user_id: NADIA_ID,
    email: 'nadia.putri@tim.id',
    name: 'Nadia Putri',
    role: 'member' as const,
    invited_by_label: 'raka@tim.id',
    added_at: '2026-10-21T08:15:00Z',
    sent_at: '2026-10-21T08:15:01Z' as string | null,
    expires_at: NADIA_EXPIRES,
    expired: false,
    delivery_status: 'sent' as const,
    email_failed: false,
    resend_count: 0,
  },
  {
    user_id: YOGA_ID,
    email: 'yoga.saputra@tim.id',
    name: 'Yoga Saputra',
    role: 'owner' as const,
    invited_by_label: 'raka@tim.id',
    added_at: '2026-10-12T03:00:00Z',
    sent_at: '2026-10-12T03:00:01Z' as string | null,
    expires_at: YOGA_EXPIRES,
    expired: true,
    delivery_status: 'sent' as const,
    email_failed: false,
    resend_count: 0,
  },
];
export const mockInvitationList = (items = mockInvitations) => ({ items });

// Baris anggota untuk tiap undangan; expires_at identik dengan baris undangannya.
const pendingMember = (i: (typeof mockInvitations)[number]) =>
  mockMember(0, {
    user_id: i.user_id,
    name: i.name,
    email: i.email,
    role: i.role,
    added_at: i.added_at,
    added_by: OWNER_ID,
    added_by_label: i.invited_by_label,
    invitation_pending: true,
    invitation_expires_at: i.expires_at,
  });

export const mockPendingMembers = mockInvitations.map(pendingMember);

export const mockInviteSent = {
  member: pendingMember(mockInvitations[1]!),
  created: true,
  email_sent: true,
  email_status: 'sent' as const,
  expires_at: NADIA_EXPIRES,
};

export const mockInviteFallback = {
  member: pendingMember(mockInvitations[0]!),
  created: true,
  email_sent: false,
  email_status: 'not_configured' as const,
  expires_at: BAGAS_EXPIRES,
  credentials: { email: 'bagas@tim.id', temporary_password: 'q7XkR2mVtz9HbNcP', login_url: 'https://grimoire.tim.id/login' },
};

export const mockResendSent = { email_sent: true, email_status: 'sent' as const, expires_at: '2026-10-28T10:30:00Z' };
export const mockResendFallback = {
  email_sent: false,
  email_status: 'failed' as const,
  expires_at: '2026-10-28T10:30:00Z',
  credentials: { email: 'bagas@tim.id', temporary_password: 'Zp4wNe8TqYv3KdRh', login_url: '/login' },
};
