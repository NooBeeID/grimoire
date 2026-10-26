import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockGroupList, mockGroups } from '../mocks/groups';
import { createGroupListStore } from '../stores/groupList.store';
import View from './ListGroupsView.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1',
  store: createGroupListStore(),
  oncreate: vi.fn(),
  onedit: vi.fn(),
  ondelete: vi.fn(),
  onprojectgone: vi.fn(),
  ...over,
});

describe('ListGroupsView', () => {
  it('is aria-busy on first load then lists terms as mono pills', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockGroupList())));
    render(View, props());
    expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(await screen.findByText('autentikasi')).toBeInTheDocument();
    expect(screen.getByText('peluncuran')).toBeInTheDocument();
  });
  it('shows the empty state for no groups', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { items: [] })));
    render(View, props());
    expect(await screen.findByText('Belum ada kelompok sinonim')).toBeInTheDocument();
  });
  it('shows an error state and retry reloads', async () => {
    const f = vi
      .fn()
      .mockResolvedValueOnce(json(500, { error: { code: 'INTERNAL', message: 'gagal' } }))
      .mockResolvedValueOnce(json(200, mockGroupList()));
    vi.stubGlobal('fetch', f);
    render(View, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByText('autentikasi')).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2);
  });
  it('404 notifies the project is gone', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x' } })));
    const p = props();
    render(View, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalledTimes(1));
  });
  it('row actions pass the group', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockGroupList())));
    const p = props();
    render(View, p);
    await userEvent.click(await screen.findByRole('button', { name: 'Ubah kelompok login, autentikasi, masuk' }));
    expect(p.onedit).toHaveBeenCalledWith(mockGroups()[0]);
    await userEvent.click(screen.getByRole('button', { name: 'Hapus kelompok deploy, rilis, peluncuran' }));
    expect(p.ondelete).toHaveBeenCalledWith(mockGroups()[1]);
  });
  it('create button calls oncreate', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, { items: [] })));
    const p = props();
    render(View, p);
    await userEvent.click(await screen.findByRole('button', { name: 'Tambah kelompok' }));
    expect(p.oncreate).toHaveBeenCalled();
  });
});
