// Data mock sesuai skema User/UserList (api/modules/core/auth.yml, kontrak T12).
// "Hari ini" = 2026-10-21 10:30 UTC, masa berlaku undangan 7 hari.
const pending = (over: Record<string, unknown>) => ({
  pending: true as const,
  invited_at: '2026-10-21T10:05:00Z',
  sent_at: null as string | null,
  expires_at: '2026-10-28T10:05:00Z',
  expired: false,
  delivery_status: 'not_configured' as 'sent' | 'failed' | 'not_configured' | 'pending',
  resend_count: 0,
  ...over,
});

const base = {
  role: 'developer' as 'developer' | 'super_admin',
  is_active: true,
  created_at: '2026-10-21T10:05:00Z',
  created_by: '0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d' as string | null,
  created_by_label: 'Sinta Admin' as string | null,
  updated_at: '2026-10-21T10:05:00Z',
  last_login_at: null as string | null,
};

export const mockAdmin = {
  ...base,
  id: '0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d',
  email: 'sinta@tim.id',
  name: 'Sinta Admin',
  role: 'super_admin' as const,
  must_change_password: false,
  created_by: null,
  created_by_label: null,
  created_at: '2026-08-01T02:00:00Z',
  updated_at: '2026-08-01T02:00:00Z',
  last_login_at: '2026-10-21T08:00:00Z',
  invitation: null,
};
export const mockRaka = {
  ...base,
  id: '9a0d4b6e-1c2f-4e3a-8b7c-6d5e4f3a2b10',
  email: 'raka@tim.id',
  name: 'Raka Pratama',
  must_change_password: false,
  created_at: '2026-09-01T02:00:00Z',
  updated_at: '2026-09-01T02:00:00Z',
  last_login_at: '2026-10-21T07:30:00Z',
  invitation: null,
};
export const mockRani = {
  ...base,
  id: 'e8b2c6a1-4d3f-4a9e-8b1c-7d6e5f4a3b21',
  email: 'rani@tim.id',
  name: 'rani',
  must_change_password: true,
  invitation: pending({}),
};
export const mockDimas = {
  ...base,
  id: '5d1c8e2a-7f3b-4a6d-9c0e-1b2a3c4d5e6f',
  email: 'dimas.anggoro@tim.id',
  name: 'Dimas Anggoro',
  must_change_password: true,
  invitation: pending({ sent_at: '2026-10-21T10:05:01Z', delivery_status: 'sent' }),
};
export const mockYoga = {
  ...base,
  id: '3a9d1e47-62c0-4f0b-8e1d-5c7a9b2f4e10',
  email: 'yoga.saputra@tim.id',
  name: 'Yoga Saputra',
  must_change_password: true,
  created_at: '2026-10-12T03:00:00Z',
  updated_at: '2026-10-12T03:00:00Z',
  invitation: pending({
    invited_at: '2026-10-12T03:00:00Z',
    sent_at: '2026-10-12T03:00:01Z',
    expires_at: '2026-10-19T03:00:00Z',
    expired: true,
    delivery_status: 'sent',
  }),
};

export const mockUsers = [mockAdmin, mockRaka, mockRani, mockDimas, mockYoga];
// Total dihitung dari baris, tidak diketik terpisah.
export const mockUserList = (items = mockUsers) => ({ items, total: items.length, limit: 20, offset: 0 });

export const mockUserInviteFallback = {
  user: mockRani,
  created: true,
  email_sent: false,
  email_status: 'not_configured' as const,
  expires_at: '2026-10-28T10:05:00Z',
  credentials: { email: 'rani@tim.id', temporary_password: 'Hm8tYq3RkW9vNcPz', login_url: 'https://grimoire.tim.id/login' },
};
export const mockUserInviteSent = {
  user: mockDimas,
  created: true,
  email_sent: true,
  email_status: 'sent' as const,
  expires_at: '2026-10-28T10:05:00Z',
};
export const mockUserResendFallback = {
  email_sent: false,
  email_status: 'failed' as const,
  expires_at: '2026-10-28T10:30:00Z',
  credentials: { email: 'rani@tim.id', temporary_password: 'Zp4wNe8TqYv3KdRh', login_url: '/login' },
};
