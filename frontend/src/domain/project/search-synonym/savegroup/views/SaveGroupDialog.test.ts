import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { SynonymGroup } from '../../listgroups';
import Dialog from './SaveGroupDialog.svelte';

const mockGroup = (n: number, over: Partial<SynonymGroup> = {}): SynonymGroup => ({
  id: `0192f5a0-3333-7aaa-8bbb-${String(n).padStart(12, '0')}`,
  terms: ['login', 'autentikasi', 'masuk'],
  created_at: '2026-10-23T10:00:00Z',
  updated_at: '2026-10-23T10:00:00Z',
  created_by_label: 'ana@tim.id',
  updated_by_label: 'ana@tim.id',
  ...over,
});


beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const props = (over: Record<string, unknown> = {}) => ({
  open: true,
  projectId: 'p1',
  group: null,
  onclose: vi.fn(),
  onsaved: vi.fn(),
  onstale: vi.fn(),
  onprojectgone: vi.fn(),
  ...over,
});
const type = async (...terms: string[]) => {
  const input = screen.getByRole('textbox');
  for (const t of terms) await userEvent.type(input, `${t}{Enter}`);
};
const save = () => screen.getByRole('button', { name: 'Simpan' });

describe('SaveGroupDialog', () => {
  it('creates a group: POST body with normalized terms, then onsaved', async () => {
    const f = vi.fn(async () => json(201, mockGroup(1)));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, p);
    await type('Login', 'autentikasi');
    await userEvent.click(save());
    await vi.waitFor(() => expect(p.onsaved).toHaveBeenCalledWith(mockGroup(1)));
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/projects/p1/search/synonyms');
    expect(JSON.parse(String(init.body))).toEqual({ terms: ['login', 'autentikasi'] });
    expect(p.onclose).toHaveBeenCalled();
  });
  it('commits the term still being typed on submit', async () => {
    const f = vi.fn(async () => json(201, mockGroup(1)));
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    await type('login');
    await userEvent.type(screen.getByRole('textbox'), 'masuk');
    await userEvent.click(save());
    await vi.waitFor(() => expect(f).toHaveBeenCalled());
    expect(JSON.parse(String((f.mock.calls[0] as unknown as [string, RequestInit])[1].body))).toEqual({ terms: ['login', 'masuk'] });
  });
  it('edits a group: PUT to the group URL, prefilled terms', async () => {
    const f = vi.fn(async () => json(200, mockGroup(1, { terms: ['login', 'autentikasi'] })));
    vi.stubGlobal('fetch', f);
    const g = mockGroup(1);
    const p = props({ group: g });
    render(Dialog, p);
    expect(screen.getByText('masuk')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Hapus term masuk' }));
    await userEvent.click(save());
    await vi.waitFor(() => expect(p.onsaved).toHaveBeenCalled());
    const [url, init] = f.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe(`/api/projects/p1/search/synonyms/${g.id}`);
    expect(init.method).toBe('PUT');
    expect(JSON.parse(String(init.body))).toEqual({ terms: ['login', 'autentikasi'] });
  });
  it('local validation: one term -> tooFew, submit disabled, no request', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    await type('login');
    expect(screen.getByText('Minimal 2 term')).toBeInTheDocument();
    expect(save()).toBeDisabled();
    await userEvent.click(save());
    expect(f).not.toHaveBeenCalled();
  });
  it('local validation: duplicate after normalization', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(Dialog, props());
    await type('Login', 'login');
    expect(screen.getByText('Ada term yang sama dalam kelompok ini')).toBeInTheDocument();
    expect(save()).toBeDisabled();
    expect(f).not.toHaveBeenCalled();
  });
  it('local validation: operator characters', async () => {
    render(Dialog, props());
    await type('login', 'a|b');
    expect(screen.getByText(/hanya boleh berisi huruf/)).toBeInTheDocument();
  });
  it('409 term_in_use names the conflicting terms and keeps the dialog open', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => json(409, { error: { code: 'CONFLICT', message: 'x', details: { reason: 'term_in_use', terms: ['login'] } } })),
    );
    const p = props();
    render(Dialog, p);
    await type('login', 'masuk');
    await userEvent.click(save());
    expect(await screen.findByText('Term sudah dipakai kelompok lain: login')).toBeInTheDocument();
    expect(p.onclose).not.toHaveBeenCalled();
  });
  it('409 group_limit shows the limit message', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(409, { error: { code: 'CONFLICT', message: 'x', details: { reason: 'group_limit' } } })));
    render(Dialog, props());
    await type('a', 'b');
    await userEvent.click(save());
    expect(await screen.findByText(/Batas jumlah kelompok/)).toBeInTheDocument();
  });
  it('404 on create -> onprojectgone', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x' } })));
    const p = props();
    render(Dialog, p);
    await type('a', 'b');
    await userEvent.click(save());
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
  it('404 on edit -> onstale', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x' } })));
    const p = props({ group: mockGroup(1) });
    render(Dialog, p);
    await userEvent.click(save());
    await vi.waitFor(() => expect(p.onstale).toHaveBeenCalled());
    expect(p.onprojectgone).not.toHaveBeenCalled();
  });
  it('500 shows the server message', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(500, { error: { code: 'INTERNAL', message: 'Server gagal' } })));
    render(Dialog, props());
    await type('a', 'b');
    await userEvent.click(save());
    expect(await screen.findByText('Server gagal')).toBeInTheDocument();
  });
});
