import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockFile } from '../../base';
import View from './MoveFileDialog.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (s: number, b: unknown) => new Response(JSON.stringify(b), { status: s });
const props = (over: Record<string, unknown> = {}) => ({ open: true, projectId: 'p1', from: 'docs/a.md', onmoved: vi.fn(), onclose: vi.fn(), onnotfound: vi.fn(), ...over });

describe('MoveFileDialog', () => {
  it('starts with the source path and disables save while unchanged', () => {
    render(View, props());
    expect(screen.getByLabelText('Path baru')).toHaveValue('docs/a.md');
    expect(screen.getByRole('button', { name: 'Pindahkan' })).toBeDisabled();
  });
  it('unsupported extension disables save', async () => {
    render(View, props());
    const input = screen.getByLabelText('Path baru');
    await userEvent.clear(input);
    await userEvent.type(input, 'docs/a.yaml');
    expect(screen.getByRole('button', { name: 'Pindahkan' })).toBeDisabled();
  });
  it('moves and reports the new file', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockFile('docs/b.md'))));
    const p = props();
    render(View, p);
    const input = screen.getByLabelText('Path baru');
    await userEvent.clear(input);
    await userEvent.type(input, 'docs/b.md');
    await userEvent.click(screen.getByRole('button', { name: 'Pindahkan' }));
    await vi.waitFor(() => expect(p.onmoved).toHaveBeenCalled());
  });
  it('409 path_exists shows an inline error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(409, { error: { code: 'CONFLICT', message: 'sudah ada', details: { reason: 'path_exists' } } })));
    render(View, props());
    const input = screen.getByLabelText('Path baru');
    await userEvent.clear(input);
    await userEvent.type(input, 'docs/b.md');
    await userEvent.click(screen.getByRole('button', { name: 'Pindahkan' }));
    expect(await screen.findByText(/sudah/i)).toBeInTheDocument();
  });
  it('404 calls onnotfound', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, { error: { code: 'NOT_FOUND', message: 'x', details: null } })));
    const p = props();
    render(View, p);
    const input = screen.getByLabelText('Path baru');
    await userEvent.clear(input);
    await userEvent.type(input, 'docs/b.md');
    await userEvent.click(screen.getByRole('button', { name: 'Pindahkan' }));
    await vi.waitFor(() => expect(p.onnotfound).toHaveBeenCalled());
  });
});
