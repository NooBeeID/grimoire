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
  created_at: '2026-10-21T09:00:00Z', created_by: null, created_by_label: null, updated_at: '2026-10-21T09:00:00Z', last_login_at: null, ...extra,
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
