import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { emptyDoc, hostileDoc, loginDoc, notesTxt } from '../mocks/files';
import Breadcrumb from './Breadcrumb.svelte';
import FileContent from './FileContent.svelte';
import FileMeta from './FileMeta.svelte';

beforeEach(() => setLocale('id'));

describe('Breadcrumb', () => {
  it('makes folder segments buttons and the last segment the current page', async () => {
    const ondir = vi.fn();
    render(Breadcrumb, { path: 'docs/auth/login.md', ondir, oncopy: vi.fn() });
    expect(screen.getByRole('navigation', { name: 'Lokasi file' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'auth' }));
    expect(ondir).toHaveBeenCalledWith('docs/auth');
    await userEvent.click(screen.getByRole('button', { name: 'docs' }));
    expect(ondir).toHaveBeenCalledWith('docs');
    expect(screen.getByText('login.md')).toHaveAttribute('aria-current', 'page');
    expect(screen.queryByRole('button', { name: 'login.md' })).not.toBeInTheDocument();
  });
  it('calls oncopy from the copy button', async () => {
    const oncopy = vi.fn();
    render(Breadcrumb, { path: 'a.md', ondir: vi.fn(), oncopy });
    await userEvent.click(screen.getByRole('button', { name: 'Salin path' }));
    expect(oncopy).toHaveBeenCalledTimes(1);
  });
  it('renders a path containing HTML literally', () => {
    render(Breadcrumb, { path: '<b>x</b>/<img src=x onerror=alert(1)>.md', ondir: vi.fn(), oncopy: vi.fn() });
    expect(document.querySelector('b, img')).toBeNull();
    expect(screen.getByText('<img src=x onerror=alert(1)>.md')).toBeInTheDocument();
  });
});

describe('FileMeta', () => {
  it('shows size, author and a machine-readable time', () => {
    render(FileMeta, { file: loginDoc });
    expect(screen.getByText(/Ukuran/)).toBeInTheDocument();
    expect(screen.getByText(/ana@tim\.id/)).toBeInTheDocument();
    expect(document.querySelector('time')).toHaveAttribute('datetime', loginDoc.updated_at);
  });
  it('renders an author label with HTML literally', () => {
    render(FileMeta, { file: { ...loginDoc, updated_by: { id: 'u', label: '<b>Budi</b>' } } });
    expect(document.querySelector('b')).toBeNull();
    expect(screen.getByText(/<b>Budi<\/b>/)).toBeInTheDocument();
  });
});

describe('FileContent', () => {
  it('renders .md through the Markdown view', () => {
    render(FileContent, { file: loginDoc });
    expect(screen.getByRole('heading', { level: 1, name: 'Alur login' })).toBeInTheDocument();
  });
  it('renders .txt as preformatted text without Markdown', () => {
    render(FileContent, { file: notesTxt });
    expect(screen.queryByRole('heading')).not.toBeInTheDocument();
    expect(document.querySelector('pre')).toHaveTextContent('# bukan heading');
  });
  it('says so for an empty file', () => {
    render(FileContent, { file: emptyDoc });
    expect(screen.getByText('File ini kosong')).toBeInTheDocument();
  });
  it('T05-AC5: hostile content yields no script element and no javascript: link', () => {
    render(FileContent, { file: hostileDoc });
    expect(document.querySelector('script')).toBeNull();
    expect(document.querySelector('a[href^="javascript"]')).toBeNull();
  });
  it('resolves relative links with the provided resolver', () => {
    const file = { ...loginDoc, content: '[sesi](./sesi.md)' };
    render(FileContent, { file, resolveLink: (h: string) => `/x/${h}` });
    expect(screen.getByRole('link', { name: 'sesi' })).toHaveAttribute('href', '/x/./sesi.md');
  });
});
