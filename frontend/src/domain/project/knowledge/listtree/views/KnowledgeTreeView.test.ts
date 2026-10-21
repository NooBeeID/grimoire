import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { authLevel, docsLevel, emptyRoot, rootLevel } from '../mocks/tree';
import View from './KnowledgeTreeView.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const levels: Record<string, unknown> = { '': rootLevel, docs: docsLevel, 'docs/auth': authLevel };
const json = (s: number, b: unknown) => new Response(JSON.stringify(b), { status: s });
const fake = () =>
  vi.fn(async (url: string) => {
    const prefix = new URL(url, 'http://x').searchParams.get('prefix') ?? '';
    return json(200, levels[prefix]);
  });
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1',
  selectedPath: null,
  refreshKey: 0,
  onopenfile: vi.fn(),
  onopendir: vi.fn(),
  onaction: vi.fn(),
  onprojectgone: vi.fn(),
  onempty: vi.fn(),
  ...over,
});

describe('KnowledgeTreeView (T05-AC1 UI)', () => {
  it('loads root, then one request per opened folder; reopening uses cache', async () => {
    const f = fake();
    vi.stubGlobal('fetch', f);
    render(View, props());
    await userEvent.click(await screen.findByRole('button', { name: /^docs/ }));
    expect(await screen.findByText('arsitektur.md')).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2);
    await userEvent.click(screen.getByRole('button', { name: /^docs/ }));
    await userEvent.click(screen.getByRole('button', { name: /^docs/ }));
    expect(f).toHaveBeenCalledTimes(2);
  });
  it('selectedPath expands ancestors in order and marks the item selected', async () => {
    const f = fake();
    vi.stubGlobal('fetch', f);
    render(View, props({ selectedPath: 'docs/auth/login.md' }));
    const item = await screen.findByRole('treeitem', { name: /login\.md/ });
    expect(item).toHaveAttribute('aria-selected', 'true');
    expect(f.mock.calls.map((c) => c[0])).toEqual([
      '/api/projects/p1/knowledge/tree',
      '/api/projects/p1/knowledge/tree?prefix=docs',
      '/api/projects/p1/knowledge/tree?prefix=docs%2Fauth',
    ]);
  });
  it('clicking a file calls onopenfile', async () => {
    vi.stubGlobal('fetch', fake());
    const p = props();
    render(View, p);
    await userEvent.click(await screen.findByRole('button', { name: 'README.md' }));
    expect(p.onopenfile).toHaveBeenCalledWith('README.md');
  });
  it('refreshKey reloads root + expanded only', async () => {
    const f = fake();
    vi.stubGlobal('fetch', f);
    const { rerender } = render(View, props({ selectedPath: 'docs/auth/login.md' }));
    await screen.findByText('login.md');
    f.mockClear();
    await rerender(props({ selectedPath: 'docs/auth/login.md', refreshKey: 1 }));
    await vi.waitFor(() => expect(f.mock.calls.map((c) => c[0])).toHaveLength(3));
  });
  it('one folder failing shows a retry row without breaking others', async () => {
    const f = vi.fn(async (url: string) =>
      url.includes('prefix=docs') ? json(500, { error: { code: 'INTERNAL', message: 'x', details: null } }) : json(200, rootLevel),
    );
    vi.stubGlobal('fetch', f);
    render(View, props());
    await userEvent.click(await screen.findByRole('button', { name: /^docs/ }));
    expect(await screen.findByText('Gagal memuat folder')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'README.md' })).toBeInTheDocument();
  });
  it('404 calls onprojectgone; empty root calls onempty(true)', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(View, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalledTimes(1));
    vi.stubGlobal('fetch', vi.fn(async () => json(200, emptyRoot)));
    const q = props();
    render(View, q);
    await vi.waitFor(() => expect(q.onempty).toHaveBeenCalledWith(true));
    expect(await screen.findByText('Belum ada file')).toBeInTheDocument();
  });
  it('file names render literally (no HTML injection)', async () => {
    const evil = { ...rootLevel, folders: [], files: [{ name: '<img src=x onerror=alert(1)>.md', path: 'e.md', size_bytes: 1, updated_at: '2026-10-21T09:30:00Z' }], total_files: 1 };
    vi.stubGlobal('fetch', vi.fn(async () => json(200, evil)));
    render(View, props());
    expect(await screen.findByText('<img src=x onerror=alert(1)>.md')).toBeInTheDocument();
    expect(document.querySelector('img')).toBeNull();
  });
});
