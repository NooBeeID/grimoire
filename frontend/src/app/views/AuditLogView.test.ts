import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import App from '../App.svelte';
import { notFoundRoute, routes } from '../routes/index';
import { startRouter } from '../routes/router';
import { setLocale } from '../../shared/stores/i18n';
import { resetSession, setIdentity, type Role } from '../../shared/stores/session';

const mockItem = (n: number) => ({
  id: `0192f0c4-7b00-7d11-8a00-${String(n).padStart(12, '0')}`,
  occurred_at: '2026-10-19T08:14:22.123456Z',
  actor_type: 'api_key',
  actor_id: '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c',
  actor_label: 'claude-desktop',
  project_id: null,
  project_label: null,
  action: 'mcp.read_file',
  target_type: 'file',
  target: 'docs/login.md',
  status: 'ok',
});
const mockList = (n: number) => ({ items: Array.from({ length: n }, (_, i) => mockItem(i + 1)), next_cursor: null });
const mockDetail = { ...mockItem(1), args: { path: 'a' }, result: { bytes: 1 }, ip: null, user_agent: null, trace_id: 't' };

let stop: () => void;
let fetchMock: ReturnType<typeof vi.fn>;
const LIST = mockList(3);
const ID = LIST.items[0]!.id;

function boot(path: string, role: Role | null) {
  fetchMock = vi.fn(async (url: string) => {
    if (url.startsWith('/api/audit-log/')) return new Response(JSON.stringify({ ...mockDetail, id: ID }), { status: 200 });
    return new Response(JSON.stringify(LIST), { status: 200 });
  });
  vi.stubGlobal('fetch', fetchMock);
  history.replaceState({}, '', path);
  stop = startRouter(routes, notFoundRoute);
  const r = render(App);
  if (role) setIdentity({ userId: 'u', email: 'e@x', name: 'N', role, mustChangePassword: false });
  else setIdentity(null);
  return r;
}
beforeEach(() => {
  setLocale('id');
  resetSession();
});
afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});

describe('AuditLogView route', () => {
  it('T02-AC5 developer gets not found, no fetch, no menu', async () => {
    boot('/audit-log', 'developer');
    expect(await screen.findByTestId('notfound-title')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.queryByRole('link', { name: 'Audit Log' })).toBeNull();
  });
  it('super admin sees title, menu with aria-current, document title', async () => {
    boot('/audit-log', 'super_admin');
    expect(await screen.findByRole('heading', { level: 1, name: 'Audit Log' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Audit Log' })).toHaveAttribute('aria-current', 'page');
    expect(document.title).toBe('Audit Log · Grimoire');
  });
  it('anonymous is redirected with query preserved', async () => {
    boot('/audit-log?action=auth.logout', null);
    await vi.waitFor(() => expect(window.location.pathname).toBe('/login'));
    expect(window.location.search).toBe('?next=%2Faudit-log%3Faction%3Dauth.logout');
  });
  it('row click pushes entry, Back closes, close removes entry and restores focus', async () => {
    boot('/audit-log', 'super_admin');
    const btn = (await screen.findAllByRole('button', { name: /\d{4}/ }))[0]!;
    const before = history.length;
    await userEvent.click(btn);
    expect(window.location.search).toBe(`?entry=${ID}`);
    expect(history.length).toBe(before + 1);
    expect(await screen.findByText('Ringkasan hasil')).toBeInTheDocument();
    history.back();
    await vi.waitFor(() => expect(window.location.search).toBe(''));
    await vi.waitFor(() => expect(screen.queryByText('Ringkasan hasil')).toBeNull());

    await userEvent.click((await screen.findAllByRole('button', { name: /\d{4}/ }))[0]!);
    await screen.findByText('Ringkasan hasil');
    const len = history.length;
    await userEvent.click(screen.getByRole('button', { name: 'Tutup' }));
    expect(window.location.search).toBe('');
    expect(history.length).toBe(len);
    await vi.waitFor(() => expect(document.activeElement).toHaveAttribute('data-entry-id', ID));
  });
  it('apply filter uses replaceState', async () => {
    boot('/audit-log', 'super_admin');
    await screen.findAllByRole('button', { name: /\d{4}/ });
    const len = history.length;
    await userEvent.selectOptions(screen.getByLabelText('Rentang waktu'), '24h');
    await userEvent.click(screen.getByRole('button', { name: 'Terapkan' }));
    expect(window.location.search).toBe('?range=24h');
    expect(history.length).toBe(len);
  });
  it('deep link sends actor params and shows chip', async () => {
    const aid = '0192f0c4-7a1e-7c3a-9b1d-3c4e5f6a7b8c';
    boot(`/audit-log?actor_type=api_key&actor_id=${aid}`, 'super_admin');
    expect(await screen.findByText('ID aktor: 0192f0c4')).toBeInTheDocument();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled());
    const url = fetchMock.mock.calls[0]![0] as string;
    expect(url).toContain('actor_type=api_key');
    expect(url).toContain(`actor_id=${aid}`);
  });
});
