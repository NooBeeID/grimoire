import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import View from './KnowledgeSearchView.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (s: number, b: unknown) => new Response(JSON.stringify(b), { status: s });
const result = {
  items: [
    { path: 'docs/auth/login.md', title: 'Alur login', score: 2, snippet: [{ text: 'Alur ', highlight: false }, { text: 'login', highlight: true }] },
    { path: 'docs/guide.md', title: 'Panduan', score: 1, snippet: [{ text: 'sebut login sekali', highlight: false }] },
  ],
  total: 5,
};
const props = (over: Record<string, unknown> = {}) => ({
  projectId: 'p1', q: 'login', scope: '', contextDir: '', onscopechange: vi.fn(), onopen: vi.fn(), onprojectgone: vi.fn(), ...over,
});

describe('KnowledgeSearchView (T05-AC4 UI)', () => {
  it('keeps the server order', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => json(200, result)));
    render(View, props());
    await screen.findByText('Alur login');
    const names = screen.getAllByRole('button').map((b) => b.textContent ?? '');
    expect(names[0]).toContain('Alur login');
    expect(names[1]).toContain('Panduan');
  });
  it('empty query sends no request', () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(View, props({ q: '   ' }));
    expect(f).not.toHaveBeenCalled();
  });
  it('no results shows empty state, not an error', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => json(200, { items: [], total: 0 })));
    render(View, props());
    expect(await screen.findByText('Tidak ada hasil')).toBeInTheDocument();
  });
  it('400 reason is shown as a message', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => json(400, { error: { code: 'INVALID_ARGUMENT', message: 'x', details: { reason: 'path_traversal' } } })));
    render(View, props({ scope: '../x' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/\./);
  });
  it('404 calls onprojectgone', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(View, p);
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
  it('"more" refetches with limit=50', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => json(200, result));
    vi.stubGlobal('fetch', f);
    render(View, props());
    await userEvent.click(await screen.findByRole('button', { name: 'Tampilkan lebih banyak' }));
    await vi.waitFor(() => expect(String(f.mock.calls.at(-1)![0])).toContain('limit=50'));
  });
  it('scope toggle reports the context dir', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => json(200, result)));
    const p = props({ contextDir: 'docs' });
    render(View, p);
    await userEvent.click(await screen.findByRole('checkbox'));
    expect(p.onscopechange).toHaveBeenCalledWith('docs');
  });
});
