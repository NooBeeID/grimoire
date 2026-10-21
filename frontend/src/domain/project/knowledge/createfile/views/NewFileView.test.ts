import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockFile } from '../../base';
import View from './NewFileView.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (s: number, b: unknown) => new Response(JSON.stringify(b), { status: s });
const err = (s: number, code: string, details: unknown) => json(s, { error: { code, message: 'pesan server', details } });
const props = (over: Record<string, unknown> = {}) => ({ projectId: 'p1', dir: '', oncreated: vi.fn(), oncancel: vi.fn(), ondirtychange: vi.fn(), onopenexisting: vi.fn(), ...over });

describe('NewFileView', () => {
  it('prefills the directory', () => {
    render(View, props({ dir: 'docs' }));
    expect(screen.getByLabelText('Path file')).toHaveValue('docs/');
  });
  it('save is disabled for an invalid path, enabled for a valid one', async () => {
    render(View, props());
    const save = screen.getByRole('button', { name: 'Simpan' });
    expect(save).toBeDisabled();
    await userEvent.type(screen.getByLabelText('Path file'), '.env');
    expect(save).toBeDisabled();
    expect(await screen.findByText(/tersembunyi|hidden/i)).toBeInTheDocument();
  });
  it('creates a file, posts {path, content} and calls oncreated', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => json(201, mockFile('docs/a.md')));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(View, p);
    await userEvent.type(screen.getByLabelText('Path file'), 'docs/a.md');
    await userEvent.type(screen.getByLabelText('Isi Markdown'), 'halo');
    expect(p.ondirtychange).toHaveBeenCalledWith(true);
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await vi.waitFor(() => expect(p.oncreated).toHaveBeenCalled());
    const body = JSON.parse((f.mock.calls[0]![1] as RequestInit).body as string);
    expect(body).toEqual({ path: 'docs/a.md', content: 'halo' });
  });
  it('409 path_exists -> inline error and open-existing link', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => err(409, 'CONFLICT', { reason: 'path_exists' })));
    const p = props();
    render(View, p);
    await userEvent.type(screen.getByLabelText('Path file'), 'docs/a.md');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Buka file yang ada' }));
    expect(p.onopenexisting).toHaveBeenCalledWith('docs/a.md');
    expect(p.oncreated).not.toHaveBeenCalled();
  });
  it('413 keeps the editor content and shows the limit', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => err(413, 'PAYLOAD_TOO_LARGE', { reason: 'too_large', max_bytes: 1048576 })));
    render(View, props());
    await userEvent.type(screen.getByLabelText('Path file'), 'a.md');
    await userEvent.type(screen.getByLabelText('Isi Markdown'), 'isi');
    await userEvent.click(screen.getByRole('button', { name: 'Simpan' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('1 MB');
    expect(screen.getByLabelText('Isi Markdown')).toHaveValue('isi');
  });
});
