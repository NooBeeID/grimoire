import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../shared/stores/i18n';
import { resetSession } from '../../../shared/stores/session';
import { startRouter } from '../../routes/router';
import Tab from './ApiSpecsTab.svelte';

const mockSpec = (over: Record<string, unknown> = {}) => ({
  name: 'payment', title: 'Payment API', openapi_version: '3.0.3', api_version: '1.4.0', format: 'yaml', source_path: 'specs/payment.yaml',
  size_bytes: 40211, endpoint_count: 1, created_at: '2026-10-23T09:00:00Z', created_by_label: 'Budi', updated_at: '2026-10-24T10:00:00Z', updated_by_label: 'Sari', ...over,
});
const mockSpecList = () => ({ items: [mockSpec({ name: 'auth', title: 'Auth API' }), mockSpec()], total: 2, limit: 100 });
const mockDetail = () => ({
  spec: mockSpec(),
  endpoints: [{ method: 'GET', path: '/payments', operation_id: 'listPayments', summary: 'Daftar pembayaran', tags: ['payments'], deprecated: false }],
  document: { openapi: '3.0.3', info: { title: 'Payment API', version: '1.4.0' }, paths: { '/payments': { get: { responses: { '200': { description: 'OK' } } } } } },
});
const PID = '0192f5a0-1111-7aaa-8bbb-000000000001';
const ctx = { project: { id: PID } } as never;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const props = (over = {}) => ({ ctx, currentUserId: 'u1', onprojectgone: vi.fn(), onleft: vi.fn(), item: null, query: new URLSearchParams(), ...over });
let stop: () => void;

beforeEach(() => {
  setLocale('id');
  resetSession();
  history.replaceState({}, '', `/projects/${PID}/api-specs`);
  stop = startRouter([]);
});
afterEach(() => {
  stop();
  vi.unstubAllGlobals();
});

describe('ApiSpecsTab', () => {
  it('list mode renders specs; 404 is forwarded', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockSpecList())));
    const { unmount } = render(Tab, props());
    expect(await screen.findByRole('link', { name: 'payment' })).toHaveAttribute('href', `/projects/${PID}/api-specs/payment`);
    unmount();
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(Tab, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
  it('detail mode renders the spec with actions and a back link', async () => {
    history.replaceState({}, '', `/projects/${PID}/api-specs/payment`);
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetail())));
    render(Tab, props({ item: 'payment' }));
    expect(await screen.findByRole('heading', { name: 'payment' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unduh' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Ubah nama' })).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: 'Kembali ke daftar spec' })[0]).toHaveAttribute('href', `/projects/${PID}/api-specs`);
  });
  it('delete navigates back to the list', async () => {
    history.replaceState({}, '', `/projects/${PID}/api-specs/payment`);
    vi.stubGlobal('fetch', vi.fn(async (_u: string, i?: RequestInit) => (i?.method === 'DELETE' ? new Response(null, { status: 204 }) : json(200, mockDetail()))));
    render(Tab, props({ item: 'payment' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Hapus' }));
    await userEvent.click(screen.getByRole('button', { name: 'Hapus spec' }));
    await vi.waitFor(() => expect(window.location.pathname).toBe(`/projects/${PID}/api-specs`));
  });
  it('rename replaces the URL with the new name', async () => {
    history.replaceState({}, '', `/projects/${PID}/api-specs/payment?op=GET%20%2Fpayments`);
    const renamed = mockSpec({ name: 'billing' });
    vi.stubGlobal('fetch', vi.fn(async (_u: string, i?: RequestInit) => (i?.method === 'PATCH' ? json(200, { spec: renamed }) : json(200, mockDetail()))));
    render(Tab, props({ item: 'payment' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Ubah nama' }));
    const input = screen.getByLabelText('Nama baru');
    await userEvent.clear(input);
    await userEvent.type(input, 'billing');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(window.location.pathname + window.location.search).toBe(`/projects/${PID}/api-specs/billing?op=GET%20%2Fpayments`));
  });

  it('list view shows Sync dari Git and opens the api_spec dialog', async () => {
    const f = vi.fn(async (u: string) =>
      u.includes('/git-sync') ? json(200, { kind: 'api_spec', feature_enabled: true, has_credential: false, credential_unreadable: false, config: null }) : json(200, mockSpecList()),
    );
    vi.stubGlobal('fetch', f);
    render(Tab, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Sync dari Git' }));
    expect(await screen.findByRole('heading', { name: 'Sync API spec dari Git' })).toBeInTheDocument();
    expect(f.mock.calls.some((c) => String(c[0]).includes('/git-sync?kind=api_spec'))).toBe(true);
  });
  it('detail view has no Sync dari Git button', async () => {
    history.replaceState({}, '', `/projects/${PID}/api-specs/payment`);
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockDetail())));
    render(Tab, props({ item: 'payment' }));
    await screen.findByRole('button', { name: 'Unduh' });
    expect(screen.queryByRole('button', { name: 'Sync dari Git' })).toBeNull();
  });
});
