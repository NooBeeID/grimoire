import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { TreeRow } from '../types';
import TreeView from './TreeView.svelte';

const rows: TreeRow[] = [
  { key: 'd:docs', kind: 'folder', path: 'docs', name: 'docs', depth: 0, expanded: true, fileCount: 3, parent: '' },
  { key: 'd:docs/auth', kind: 'folder', path: 'docs/auth', name: 'auth', depth: 1, expanded: false, fileCount: 2, parent: 'docs' },
  { key: 'f:docs/a.md', kind: 'file', path: 'docs/a.md', name: 'a.md', depth: 1, parent: 'docs' },
  { key: 'f:README.md', kind: 'file', path: 'README.md', name: 'README.md', depth: 0, parent: '' },
];

const props = (over: Record<string, unknown> = {}) => ({
  rows,
  selected: null,
  onselect: vi.fn(),
  ontoggle: vi.fn(),
  onaction: vi.fn(),
  onretry: vi.fn(),
  ...over,
});
const btn = (path: string) => screen.getByTitle(path);

beforeEach(() => setLocale('id'));

describe('TreeView roles and state', () => {
  it('exposes tree and treeitem roles with level, expanded and selected', () => {
    render(TreeView, props({ selected: 'docs/a.md' }));
    expect(screen.getByRole('tree', { name: 'Pohon file knowledge' })).toBeInTheDocument();
    const items = screen.getAllByRole('treeitem');
    expect(items).toHaveLength(4);
    expect(items[0]).toHaveAttribute('aria-level', '1');
    expect(items[0]).toHaveAttribute('aria-expanded', 'true');
    expect(items[1]).toHaveAttribute('aria-level', '2');
    expect(items[1]).toHaveAttribute('aria-expanded', 'false');
    expect(items[2]).not.toHaveAttribute('aria-expanded');
    expect(items[2]).toHaveAttribute('aria-selected', 'true');
    expect(items[3]).toHaveAttribute('aria-selected', 'false');
  });
  it('shows the file count on folders only', () => {
    render(TreeView, props());
    expect(screen.getByText('3 file')).toBeInTheDocument();
    expect(screen.getByText('2 file')).toBeInTheDocument();
  });
  it('uses a roving tabindex with one tab stop', () => {
    render(TreeView, props());
    const stops = screen.getAllByTitle(/./).filter((b) => b.getAttribute('tabindex') === '0');
    expect(stops).toHaveLength(1);
  });
  it('renders a hostile file name literally and keeps the full path in the title', () => {
    const hostile: TreeRow = { key: 'f:x', kind: 'file', path: 'docs/<img src=x onerror=alert(1)>.md', name: '<img src=x onerror=alert(1)>.md', depth: 0, parent: 'docs' };
    render(TreeView, props({ rows: [hostile] }));
    expect(document.querySelector('img')).toBeNull();
    expect(screen.getByText('<img src=x onerror=alert(1)>.md')).toBeInTheDocument();
    expect(btn('docs/<img src=x onerror=alert(1)>.md')).toBeInTheDocument();
  });
  it('shows loading, error with retry, and empty rows', async () => {
    const extra: TreeRow[] = [
      { key: 'l', kind: 'loading', path: 'a', name: '', depth: 1, parent: 'a' },
      { key: 'e', kind: 'error', path: 'b', name: '', depth: 1, parent: 'b' },
      { key: 'm', kind: 'empty', path: 'c', name: '', depth: 1, parent: 'c' },
    ];
    const p = props({ rows: extra });
    render(TreeView, p);
    expect(screen.getByText('Memuat...')).toHaveAttribute('aria-busy', 'true');
    expect(screen.getByText('Folder kosong')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(p.onretry).toHaveBeenCalledWith('b');
  });
});

describe('TreeView interaction', () => {
  it('selects a file on click and toggles then selects a folder', async () => {
    const p = props();
    render(TreeView, p);
    await userEvent.click(btn('README.md'));
    expect(p.onselect).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'file', path: 'README.md' }));
    await userEvent.click(btn('docs'));
    expect(p.ontoggle).toHaveBeenCalledWith('docs');
    expect(p.onselect).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'folder', path: 'docs', fileCount: 3 }));
  });

  it('moves focus with ArrowDown, ArrowUp, Home and End', async () => {
    render(TreeView, props());
    btn('docs').focus();
    await userEvent.keyboard('{ArrowDown}');
    await vi.waitFor(() => expect(btn('docs/auth')).toHaveFocus());
    await userEvent.keyboard('{ArrowDown}');
    await vi.waitFor(() => expect(btn('docs/a.md')).toHaveFocus());
    await userEvent.keyboard('{ArrowUp}');
    await vi.waitFor(() => expect(btn('docs/auth')).toHaveFocus());
    await userEvent.keyboard('{End}');
    await vi.waitFor(() => expect(btn('README.md')).toHaveFocus());
    await userEvent.keyboard('{Home}');
    await vi.waitFor(() => expect(btn('docs')).toHaveFocus());
  });

  it('ArrowRight opens a closed folder and enters an open one; ArrowLeft closes or goes to the parent', async () => {
    const p = props();
    render(TreeView, p);
    btn('docs/auth').focus();
    await userEvent.keyboard('{ArrowRight}');
    expect(p.ontoggle).toHaveBeenCalledWith('docs/auth');
    await userEvent.keyboard('{ArrowLeft}');
    await vi.waitFor(() => expect(btn('docs')).toHaveFocus());
    await userEvent.keyboard('{ArrowRight}');
    await vi.waitFor(() => expect(btn('docs/auth')).toHaveFocus());
    btn('docs').focus();
    p.ontoggle.mockClear();
    await userEvent.keyboard('{ArrowLeft}');
    expect(p.ontoggle).toHaveBeenCalledWith('docs');
  });

  it('Enter and Space activate the focused item', async () => {
    const p = props();
    render(TreeView, p);
    btn('README.md').focus();
    await userEvent.keyboard('{Enter}');
    expect(p.onselect).toHaveBeenCalledTimes(1);
    await userEvent.keyboard(' ');
    expect(p.onselect).toHaveBeenCalledTimes(2);
  });
});

describe('TreeView node menu', () => {
  it('offers folder actions for a folder and file actions for a file', async () => {
    render(TreeView, props());
    await userEvent.click(screen.getByRole('button', { name: 'Aksi untuk docs' }));
    expect(screen.getAllByRole('menuitem').map((m) => m.textContent?.trim())).toEqual([
      'File baru di sini',
      'Upload ke sini',
      'Salin path',
      'Hapus folder',
    ]);
    await userEvent.click(screen.getByRole('button', { name: 'Aksi untuk README.md' }));
    expect(screen.getAllByRole('menuitem').map((m) => m.textContent?.trim())).toEqual(['Buka', 'Pindah atau ganti nama', 'Salin path', 'Hapus']);
  });

  it('reports the chosen action with the node and closes the menu', async () => {
    const p = props();
    render(TreeView, p);
    await userEvent.click(screen.getByRole('button', { name: 'Aksi untuk docs' }));
    await userEvent.click(screen.getByRole('menuitem', { name: 'Hapus folder' }));
    expect(p.onaction).toHaveBeenCalledWith(expect.objectContaining({ kind: 'folder', path: 'docs', fileCount: 3 }), 'deleteFolder');
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });

  it('Escape closes the menu and returns focus to its button', async () => {
    render(TreeView, props());
    const trigger = screen.getByRole('button', { name: 'Aksi untuk README.md' });
    await userEvent.click(trigger);
    await vi.waitFor(() => expect(screen.getAllByRole('menuitem')[0]).toHaveFocus());
    await userEvent.keyboard('{Escape}');
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
  });

  it('arrow keys cycle through the menu items', async () => {
    render(TreeView, props());
    await userEvent.click(screen.getByRole('button', { name: 'Aksi untuk README.md' }));
    const items = screen.getAllByRole('menuitem');
    await vi.waitFor(() => expect(items[0]).toHaveFocus());
    await userEvent.keyboard('{ArrowDown}');
    expect(items[1]).toHaveFocus();
    await userEvent.keyboard('{ArrowUp}{ArrowUp}');
    expect(items[3]).toHaveFocus();
  });
});
