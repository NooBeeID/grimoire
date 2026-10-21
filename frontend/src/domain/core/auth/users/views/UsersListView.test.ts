import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { resetSession, setIdentity } from '../../../../../shared/stores/session';
import { toast } from '../../../../../shared/stores/toast';
import { usersListStore } from '../stores/usersList';
import UsersListView from './UsersListView.svelte';

const ME = '0b7c6a4e-2f1d-4c8a-9e3b-5d6f7a8b9c0d';
const mk = (id: string, name: string, extra: Record<string, unknown> = {}) => ({
  id, email: `${name.toLowerCase()}@tim.id`, name, role: 'developer', is_active: true, must_change_password: false,
  created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null, updated_at: '2026-10-21T09:00:00Z', last_login_at: null, invitation: null, ...extra,
});
const ADMIN = mk(ME, 'Admin', { role: 'super_admin' });
const SARI = mk('7d1e0a52-3c4b-4f6a-8b9c-1d2e3f4a5b6c', 'Sari');
const json = (b: unknown, s = 200) => new Response(JSON.stringify(b), { status: s });
const list = (items: unknown[]) => json({ items, total: items.length, limit: 20, offset: 0 });

const fetchMock = vi.fn();
let routes: (url: string, init: RequestInit) => Response;

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
  fetchMock.mockImplementation(async (url: string, init: RequestInit) => routes(url, init ?? {}));
  usersListStore.reset();
  toast.clear();
  setIdentity({ userId: ME, email: 'admin@tim.id', name: 'Admin', role: 'super_admin', mustChangePassword: false });
});
afterEach(() => {
  vi.unstubAllGlobals();
  resetSession();
});

describe('UsersListView', () => {
  it('loads and renders rows; self actions disabled', async () => {
    routes = () => list([ADMIN, SARI]);
    render(UsersListView);
    expect(await screen.findByText('Sari')).toBeInTheDocument();
    const adminRow = screen.getByText('Admin').closest('tr')!;
    expect(within(adminRow).getByRole('button', { name: 'Nonaktifkan' })).toBeDisabled();
    const sariRow = screen.getByText('Sari').closest('tr')!;
    expect(within(sariRow).getByRole('button', { name: 'Nonaktifkan' })).toBeEnabled();
  });
  it('empty state without filters offers create', async () => {
    routes = () => list([]);
    render(UsersListView);
    expect(await screen.findByText('Belum ada user')).toBeInTheDocument();
  });
  it('error state shows retry', async () => {
    routes = () => json({ error: { code: 'INTERNAL', message: 'gagal muat' } }, 500);
    render(UsersListView);
    expect(await screen.findByText('gagal muat')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeInTheDocument();
  });
  it('create with generated password shows secret once, then clears it on close', async () => {
    routes = (url, init) =>
      init.method === 'POST' ? json({ user: SARI, temporary_password: 'Kq7mZp3RtX9aHw2N' }, 201) : list([ADMIN]);
    render(UsersListView);
    await screen.findByText('Admin');
    await userEvent.click(screen.getAllByRole('button', { name: 'Buat user' })[0]!);
    await userEvent.type(screen.getByLabelText('Email'), 'sari@tim.id');
    await userEvent.type(screen.getByLabelText('Nama'), 'Sari');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    const secret = (await screen.findByLabelText('Password sementara')) as HTMLInputElement;
    expect(secret.value).toBe('Kq7mZp3RtX9aHw2N');
    await userEvent.click(screen.getByRole('button', { name: 'Saya sudah menyimpan password' }));
    expect(document.body.innerHTML).not.toContain('Kq7mZp3RtX9aHw2N');
    expect(JSON.stringify(localStorage)).not.toContain('Kq7mZp3RtX9aHw2N');
  });
  it('create conflict puts message on email field', async () => {
    routes = (url, init) => (init.method === 'POST' ? json({ error: { code: 'CONFLICT', message: 'x' } }, 409) : list([ADMIN]));
    render(UsersListView);
    await screen.findByText('Admin');
    await userEvent.click(screen.getAllByRole('button', { name: 'Buat user' })[0]!);
    await userEvent.type(screen.getByLabelText('Email'), 'sari@tim.id');
    await userEvent.type(screen.getByLabelText('Nama'), 'Sari');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(await screen.findByText('Email sudah terdaftar')).toBeInTheDocument();
  });
  it('deactivates a user via confirm dialog and refreshes', async () => {
    let active = true;
    routes = (url, init) => {
      if (init.method === 'POST') {
        active = false;
        return json({ ...SARI, is_active: false });
      }
      return list([ADMIN, { ...SARI, is_active: active }]);
    };
    render(UsersListView);
    const row = (await screen.findByText('Sari')).closest('tr')!;
    await userEvent.click(within(row).getByRole('button', { name: 'Nonaktifkan' }));
    const dialog = screen.getByRole('dialog', { name: 'Nonaktifkan user' });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Nonaktifkan', hidden: true }));
    await vi.waitFor(() => expect(within(screen.getByText('Sari').closest('tr')!).getByText('Nonaktif')).toBeInTheDocument());
    expect(fetchMock.mock.calls.some(([u]) => String(u).endsWith('/deactivate'))).toBe(true);
  });
  it('last super admin conflict is shown in the dialog', async () => {
    routes = (url, init) =>
      init.method === 'POST' ? json({ error: { code: 'CONFLICT', message: 'x', details: { reason: 'LAST_SUPER_ADMIN' } } }, 409) : list([ADMIN, SARI]);
    render(UsersListView);
    const row = (await screen.findByText('Sari')).closest('tr')!;
    await userEvent.click(within(row).getByRole('button', { name: 'Nonaktifkan' }));
    const dialog = screen.getByRole('dialog', { name: 'Nonaktifkan user' });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Nonaktifkan', hidden: true }));
    expect(await within(dialog).findByText('Minimal harus ada satu super admin aktif', {}, { timeout: 2000 })).toBeInTheDocument();
  });
  it('edit loads detail then saves only diff', async () => {
    routes = (url, init) => {
      if (init.method === 'PATCH') return json({ ...SARI, name: 'Sari W.' });
      if (/\/api\/users\/[^/?]+$/.test(url)) return json(SARI);
      return list([ADMIN, SARI]);
    };
    render(UsersListView);
    const row = (await screen.findByText('Sari')).closest('tr')!;
    await userEvent.click(within(row).getByRole('button', { name: 'Ubah' }));
    const dialog = screen.getByRole('dialog', { name: 'Ubah user' });
    const name = await within(dialog).findByLabelText('Nama');
    await userEvent.clear(name);
    await userEvent.type(name, 'Sari W.');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => {
      const patch = fetchMock.mock.calls.find(([, i]) => i?.method === 'PATCH')!;
      expect(JSON.parse(patch[1].body)).toEqual({ name: 'Sari W.' });
    });
  });
});

describe('UsersListView: undangan', () => {
  const RANI = mk('e8b2c6a1-4d3f-4a9e-8b1c-7d6e5f4a3b21', 'rani', {
    must_change_password: true,
    invitation: { pending: true, invited_at: '2026-10-21T10:05:00Z', sent_at: null, expires_at: '2026-10-28T10:05:00Z', expired: false, delivery_status: 'not_configured', resend_count: 0 },
  });
  const invited = (over: Record<string, unknown> = {}) => ({
    user: RANI, created: true, email_sent: true, email_status: 'sent', expires_at: '2026-10-28T10:05:00Z', ...over,
  });
  const creds = { email: 'rani@tim.id', temporary_password: 'Hm8tYq3RkW9vNcPz', login_url: 'https://grimoire.tim.id/login' };
  const posts = () => fetchMock.mock.calls.filter((c) => (c[1] as RequestInit | undefined)?.method === 'POST');

  async function openInvite() {
    render(UsersListView, { emailEnabled: false });
    await screen.findByText('Admin');
    await userEvent.click(screen.getByRole('button', { name: 'Undang developer' }));
    const dlg = screen.getByRole('dialog');
    await userEvent.type(within(dlg).getByLabelText(/^Email/), 'rani@tim.id');
    return dlg;
  }

  it('Undang developer adalah satu-satunya tombol primary di title row; Buat user sekunder', async () => {
    routes = () => list([ADMIN]);
    render(UsersListView, { emailEnabled: false });
    await screen.findByText('Admin');
    expect(screen.getByRole('button', { name: 'Undang developer' })).toHaveClass('bg-primary');
    expect(screen.getByRole('button', { name: 'Buat user' })).not.toHaveClass('bg-primary');
  });
  it('terkirim: toast, dialog tertutup, daftar di-refresh, tanpa dialog kredensial', async () => {
    routes = (_u, init) => (init.method === 'POST' ? json(invited(), 201) : list([ADMIN]));
    const dlg = await openInvite();
    await userEvent.click(within(dlg).getByRole('button', { name: 'Kirim undangan' }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(screen.queryByTestId('temporary-password')).toBeNull();
    expect(posts()).toHaveLength(1);
  });
  it('fallback: dialog kredensial berisi password; tutup menghapusnya dari DOM', async () => {
    routes = (_u, init) =>
      init.method === 'POST' ? json(invited({ email_sent: false, email_status: 'not_configured', credentials: creds }), 201) : list([ADMIN]);
    const dlg = await openInvite();
    await userEvent.click(within(dlg).getByRole('button', { name: 'Kirim undangan' }));
    expect(await screen.findByTestId('temporary-password')).toHaveValue('Hm8tYq3RkW9vNcPz');
    await userEvent.click(screen.getByRole('button', { name: 'Saya sudah menyalin' }));
    await vi.waitFor(() => expect(document.body.innerHTML).not.toContain('Hm8tYq3RkW9vNcPz'));
  });
  it('409 registered: pesan di field email dan Tampilkan di daftar mengisi filter', async () => {
    routes = (_u, init) =>
      init.method === 'POST' ? json({ error: { code: 'ALREADY_REGISTERED', message: 'x', details: { reason: 'registered' } } }, 409) : list([ADMIN]);
    const dlg = await openInvite();
    await userEvent.click(within(dlg).getByRole('button', { name: 'Kirim undangan' }));
    expect(await within(dlg).findByText(/sudah punya akun/)).toBeInTheDocument();
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tampilkan di daftar' }));
    await vi.waitFor(() => expect(fetchMock.mock.calls.some((c) => String(c[0]).includes('q=rani%40tim.id'))).toBe(true));
  });
  it('429: pesan menit, dialog tetap terbuka', async () => {
    routes = (_u, init) =>
      init.method === 'POST' ? json({ error: { code: 'RATE_LIMITED', message: 'x', details: { retry_after_seconds: 1840, scope: 'invite' } } }, 429) : list([ADMIN]);
    const dlg = await openInvite();
    await userEvent.click(within(dlg).getByRole('button', { name: 'Kirim undangan' }));
    expect(await within(dlg).findByText(/31 menit/)).toBeInTheDocument();
  });
  it('kirim ulang dari baris: konfirmasi, fallback membuka dialog kredensial', async () => {
    routes = (url, init) =>
      init.method === 'POST' && url.includes('/resend-invitation')
        ? json({ email_sent: false, email_status: 'failed', expires_at: '2026-10-28T10:30:00Z', credentials: { ...creds, temporary_password: 'Zp4wNe8TqYv3KdRh' } })
        : list([ADMIN, RANI]);
    render(UsersListView, { emailEnabled: false });
    const row = (await screen.findByText('rani')).closest('tr')!;
    await userEvent.click(within(row).getByRole('button', { name: 'Kirim ulang undangan' }));
    await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Kirim ulang' }));
    expect(await screen.findByTestId('temporary-password')).toHaveValue('Zp4wNe8TqYv3KdRh');
  });
  it('klik ganda Kirim undangan hanya satu POST', async () => {
    routes = (_u, init) => (init.method === 'POST' ? json(invited(), 201) : list([ADMIN]));
    const dlg = await openInvite();
    const btn = within(dlg).getByRole('button', { name: 'Kirim undangan' });
    await userEvent.dblClick(btn);
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(posts()).toHaveLength(1);
  });
});
