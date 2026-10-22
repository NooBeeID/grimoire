import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockKey, mockKeyList, mockTeamKeys } from '../mocks/keys';
import { createKeyListStore } from '../stores/keyList.store';
import View from './ListKeysView.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1',
  store: createKeyListStore(),
  oncreate: vi.fn(),
  onrevoke: vi.fn(),
  onprojectgone: vi.fn(),
  ...over,
});

describe('ListKeysView', () => {
  it('developer sees own-keys intro, no owner column, active count from rows', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockKeyList(...mockTeamKeys().slice(0, 2)))));
    render(View, props());
    await screen.findByText('claude-code-ana');
    expect(screen.getByText(/Key di sini hanya milik Anda\./)).toBeInTheDocument();
    expect(screen.queryByRole('columnheader', { name: 'Dibuat oleh' })).toBeNull();
    expect(screen.getByText('2 key aktif')).toBeInTheDocument();
  });
  it('super admin sees admin intro, owner column, and 4 active keys', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockKeyList(...mockTeamKeys()))));
    render(View, props({ isSuperAdmin: true, showOwner: true }));
    await screen.findByText('cursor-budi');
    expect(screen.getByText(/Sebagai Super Admin Anda melihat key semua anggota project\./)).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Dibuat oleh' })).toBeInTheDocument();
    expect(screen.getByText('4 key aktif')).toBeInTheDocument();
  });
  it('empty state for developer mentions other members hint', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { items: [] })));
    render(View, props());
    expect(await screen.findByText(/Key yang dibuat anggota lain tidak ditampilkan di sini/)).toBeInTheDocument();
  });
  it('empty state for super admin is project-wide', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { items: [] })));
    render(View, props({ isSuperAdmin: true, showOwner: true }));
    expect(await screen.findByText('Belum ada API key di project ini')).toBeInTheDocument();
  });
  it('first load is aria-busy then lists keys with active count', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockKeyList(mockKey(1)))));
    render(View, props());
    expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(await screen.findByText('ci-pipeline')).toBeInTheDocument();
    expect(screen.getByText('1 key aktif')).toBeInTheDocument();
  });
  it('empty state with create action', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { items: [] })));
    const p = props();
    render(View, p);
    expect(await screen.findByText('Belum ada API key Anda')).toBeInTheDocument();
    await userEvent.click(screen.getAllByRole('button', { name: 'Buat key' })[1]!);
    expect(p.oncreate).toHaveBeenCalled();
  });
  it('error shows trace id and retry refetches', async () => {
    const f = vi
      .fn()
      .mockResolvedValueOnce(json(500, { error: { code: 'INTERNAL', message: 'gagal', details: null }, trace_id: 'T-1' }))
      .mockResolvedValue(json(200, mockKeyList(mockKey(1))));
    vi.stubGlobal('fetch', f);
    render(View, props());
    expect(await screen.findByText('gagal')).toBeInTheDocument();
    expect(screen.getByText(/T-1/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByText('ci-pipeline')).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2);
  });
  it('404 calls onprojectgone once', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(View, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalledTimes(1));
  });
  it('invalidate refetches and keeps old rows visible meanwhile', async () => {
    let release: (r: Response) => void = () => {};
    const f = vi
      .fn()
      .mockResolvedValueOnce(json(200, mockKeyList(mockKey(1))))
      .mockImplementationOnce(() => new Promise<Response>((r) => (release = r)));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(View, p);
    await screen.findByText('ci-pipeline');
    p.store.invalidate();
    await vi.waitFor(() => expect(f).toHaveBeenCalledTimes(2));
    expect(screen.getByText('ci-pipeline')).toBeInTheDocument();
    release(json(200, mockKeyList(mockKey(1), mockKey(2, { name: 'baru-key' }))));
    expect(await screen.findByText('baru-key')).toBeInTheDocument();
  });
});
