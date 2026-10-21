import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockFile } from '../../base';
import View from './FileViewerView.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (s: number, b: unknown) => new Response(JSON.stringify(b), { status: s });
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1',
  path: 'docs/auth/login.md',
  onedit: vi.fn(),
  onmove: vi.fn(),
  ondelete: vi.fn(),
  ondir: vi.fn(),
  onnotfound: vi.fn(),
  ...over,
});

describe('FileViewerView', () => {
  it('loading then ready: title, meta, actions, breadcrumb', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockFile('docs/auth/login.md'))));
    const p = props();
    render(View, p);
    expect(document.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(await screen.findByRole('heading', { name: 'Alur login', level: 2 })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'docs' }));
    expect(p.ondir).toHaveBeenCalledWith('docs');
    await userEvent.click(screen.getByRole('button', { name: 'auth' }));
    expect(p.ondir).toHaveBeenCalledWith('docs/auth');
  });
  it('empty title falls back to the file name', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockFile('docs/x.md', { title: '' }))));
    render(View, props({ path: 'docs/x.md' }));
    expect(await screen.findByRole('heading', { name: 'x.md' })).toBeInTheDocument();
  });
  it('404 shows notFound and calls onnotfound once', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(View, p);
    expect(await screen.findByText('File tidak ditemukan')).toBeInTheDocument();
    expect(p.onnotfound).toHaveBeenCalledTimes(1);
  });
  it('500 shows ErrorState and retry refetches', async () => {
    const f = vi
      .fn()
      .mockResolvedValueOnce(json(500, { error: { code: 'INTERNAL', message: 'gagal', details: null } }))
      .mockResolvedValue(json(200, mockFile('docs/auth/login.md')));
    vi.stubGlobal('fetch', f);
    render(View, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByRole('heading', { name: 'Alur login', level: 2 })).toBeInTheDocument();
    expect(f).toHaveBeenCalledTimes(2);
  });
  it('T05-AC5: script and javascript: links never reach the DOM', async () => {
    const content = '<script>alert(1)</script>\n\n[x](javascript:alert(1))\n\n<img src=x onerror=alert(1)>';
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockFile('xss.md', { content }))));
    render(View, props({ path: 'xss.md' }));
    await screen.findByRole('heading', { name: 'Alur login', level: 2 });
    expect(document.querySelector('script')).toBeNull();
    expect(document.querySelector('a[href^="javascript"]')).toBeNull();
    expect(document.querySelector('img')).toBeNull();
    expect(document.querySelector('[onerror]')).toBeNull();
  });
  it('.txt renders as preformatted text, not markdown', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockFile('n.txt', { content: '# bukan judul' }))));
    render(View, props({ path: 'n.txt' }));
    expect((await screen.findByText('# bukan judul')).tagName).toBe('PRE');
  });
  it('relative links use resolveLink', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockFile('docs/a.md', { content: '[b](b.md)' }))));
    const resolveLink = vi.fn(() => '/projects/p1/knowledge?path=docs%2Fb.md');
    render(View, props({ path: 'docs/a.md', resolveLink }));
    expect(await screen.findByRole('link', { name: 'b' })).toHaveAttribute('href', '/projects/p1/knowledge?path=docs%2Fb.md');
    expect(resolveLink).toHaveBeenCalledWith('b.md', 'docs');
  });
});
