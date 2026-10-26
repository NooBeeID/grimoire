import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../shared/stores/i18n';
import { findTab } from '../../workspace/tabs';
import type { SynonymGroup } from '../../../domain/project/search-synonym/listgroups';
import Tab from './SearchTab.svelte';

const mockGroup = (n: number, over: Partial<SynonymGroup> = {}): SynonymGroup => ({
  id: `0192f5a0-3333-7aaa-8bbb-${String(n).padStart(12, '0')}`,
  terms: ['login', 'autentikasi', 'masuk'],
  created_at: '2026-10-23T10:00:00Z',
  updated_at: '2026-10-23T10:00:00Z',
  created_by_label: 'ana@tim.id',
  updated_by_label: 'ana@tim.id',
  ...over,
});

const mockGroupList = () => ({ items: [mockGroup(1), mockGroup(2, { terms: ['deploy', 'rilis', 'peluncuran'], created_at: '2026-10-23T11:20:00Z' })] });

const PID = '0192f5a0-1111-7aaa-8bbb-000000000001';
const ctx = { project: { id: PID } } as never;
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const props = () => ({ ctx, currentUserId: 'u1', onprojectgone: vi.fn(), onleft: vi.fn() });

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

describe('SearchTab', () => {
  it('is registered in the workspace tabs', () => {
    expect(findTab('search')).not.toBeNull();
  });
  it('loads groups for the project', async () => {
    const f = vi.fn(async () => json(200, mockGroupList()));
    vi.stubGlobal('fetch', f);
    render(Tab, props());
    expect(await screen.findByText('autentikasi')).toBeInTheDocument();
    expect((f.mock.calls[0] as unknown as [string])[0]).toBe(`/api/projects/${PID}/search/synonyms`);
  });
  it('add flow closes the dialog and shows the new group', async () => {
    const created = mockGroup(9, { terms: ['rollback', 'pembatalan'], created_at: '2026-10-25T08:00:00Z' });
    const f = vi.fn(async (_url: string, init?: RequestInit) =>
      init?.method === 'POST' ? json(201, created) : json(200, { items: [] }),
    );
    vi.stubGlobal('fetch', f);
    render(Tab, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Tambah kelompok' }));
    const input = await screen.findByLabelText('Term setara');
    await userEvent.type(input, 'rollback{Enter}pembatalan{Enter}');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(await screen.findByRole('button', { name: 'Ubah kelompok rollback, pembatalan' })).toBeInTheDocument();
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });
  it('delete flow removes the row', async () => {
    const f = vi.fn(async (_url: string, init?: RequestInit) =>
      init?.method === 'DELETE' ? new Response(null, { status: 204 }) : json(200, mockGroupList()),
    );
    vi.stubGlobal('fetch', f);
    render(Tab, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Hapus kelompok deploy, rilis, peluncuran' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Hapus kelompok' }));
    await vi.waitFor(() => expect(screen.queryByText('peluncuran')).toBeNull());
  });
});
