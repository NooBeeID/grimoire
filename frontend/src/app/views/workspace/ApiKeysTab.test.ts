import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../shared/stores/i18n';
import { resetSession, setIdentity } from '../../../shared/stores/session';
import Tab from './ApiKeysTab.svelte';

const mockKey = (n: number) => ({
  id: `0192f5a0-2222-7aaa-8bbb-${String(n).padStart(12, '0')}`,
  name: 'ci-pipeline',
  prefix: 'a1b2c3d4',
  status: 'active',
  created_by: '0192f5a0-aaaa-7aaa-8bbb-00000000000a',
  created_by_label: 'ana@tim.id',
  created_at: '2026-10-23T10:00:00Z',
  last_used_at: null,
  revoked_at: null,
  revoked_by: null,
  revoked_by_label: null,
});
const mockKeyList = (...items: ReturnType<typeof mockKey>[]) => ({ items });
const mockCreated = () => ({
  key: { ...mockKey(1), name: 'claude-code-budi', prefix: 'k3x9q2ma' },
  raw_key: 'gr_k3x9q2ma_' + 'A'.repeat(43),
  mcp_url: null,
});
const PID = '0192f5a0-1111-7aaa-8bbb-000000000001';
const ctx = { project: { id: PID } } as never;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const props = () => ({ ctx, currentUserId: 'u1', onprojectgone: vi.fn(), onleft: vi.fn() });
const login = (role: 'developer' | 'super_admin') =>
  setIdentity({ userId: 'u1', email: 'a@tim.id', name: 'Ana', role, mustChangePassword: false });

beforeEach(() => {
  setLocale('id');
  resetSession();
});
afterEach(() => vi.unstubAllGlobals());

describe('ApiKeysTab', () => {
  it('developer: no owner column, own-keys intro; super_admin: owner column and admin intro', async () => {
    login('developer');
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockKeyList(mockKey(1)))));
    const { unmount } = render(Tab, props());
    await screen.findByText('ci-pipeline');
    expect(screen.queryByRole('columnheader', { name: 'Dibuat oleh' })).toBeNull();
    expect(screen.getByText(/hanya milik Anda/)).toBeInTheDocument();
    unmount();
    login('super_admin');
    render(Tab, props());
    await screen.findByText('ci-pipeline');
    expect(screen.getByRole('columnheader', { name: 'Dibuat oleh' })).toBeInTheDocument();
    expect(screen.getAllByText('ana@tim.id').length).toBeGreaterThan(0);
    expect(screen.getByText(/Sebagai Super Admin/)).toBeInTheDocument();
  });
  it('developer sees no audit link; super_admin gets the filtered href', async () => {
    login('developer');
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockKeyList(mockKey(1)))));
    const { unmount } = render(Tab, props());
    await screen.findByText('ci-pipeline');
    expect(screen.queryByRole('link')).toBeNull();
    unmount();
    login('super_admin');
    render(Tab, props());
    await screen.findByText('ci-pipeline');
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      `/audit-log?project_id=${PID}&action=apikey.created&action=apikey.revoked&target=${mockKey(1).id}`,
    );
  });
  it('create applies the new row without a second GET', async () => {
    login('developer');
    const f = vi.fn(async (_u: string, i?: RequestInit) =>
      i?.method === 'POST' ? json(201, mockCreated()) : json(200, { items: [] }),
    );
    vi.stubGlobal('fetch', f);
    render(Tab, props());
    await screen.findByText('Belum ada API key Anda');
    await userEvent.click(screen.getAllByRole('button', { name: 'Buat key' })[0]!);
    await userEvent.type(screen.getByLabelText('Nama key'), 'claude-code-budi');
    await userEvent.click(screen.getByRole('button', { name: 'Buat' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Saya sudah menyimpan key' }));
    expect(await screen.findByText('claude-code-budi')).toBeInTheDocument();
    expect(f.mock.calls.filter(([, i]) => !i?.method || i.method === 'GET')).toHaveLength(1);
  });
  it('revoke moves the row to revoked without a second GET', async () => {
    login('developer');
    const k = mockKey(3);
    const f = vi.fn(async (_u: string, i?: RequestInit) =>
      i?.method === 'POST'
        ? json(200, { key: { ...k, status: 'revoked', revoked_at: '2026-10-23T12:00:00Z', revoked_by_label: 'a@tim.id' } })
        : json(200, mockKeyList(k)),
    );
    vi.stubGlobal('fetch', f);
    const { container } = render(Tab, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Cabut key ci-pipeline' }));
    await userEvent.click(screen.getByRole('button', { name: 'Cabut key' }));
    await vi.waitFor(() => expect(container.querySelector('tr[data-status="revoked"]')).not.toBeNull());
    expect(f.mock.calls.filter(([, i]) => !i?.method || i.method === 'GET')).toHaveLength(1);
  });
  it('404 from list is forwarded', async () => {
    login('developer');
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(Tab, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
});
