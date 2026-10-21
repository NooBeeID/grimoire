import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import View from './DeleteFileDialog.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const props = (over: Record<string, unknown> = {}) => ({ open: true, projectId: 'p1', path: 'docs/a.md', ondeleted: vi.fn(), onclose: vi.fn(), ...over });

describe('DeleteFileDialog', () => {
  it('cancel sends no request', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    const p = props();
    render(View, p);
    await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
    expect(f).not.toHaveBeenCalled();
    expect(p.onclose).toHaveBeenCalled();
  });
  it('confirm sends DELETE and reports', async () => {
    const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(View, p);
    await userEvent.click(screen.getByRole('button', { name: 'Hapus' }));
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalledWith('docs/a.md'));
    expect((f.mock.calls[0]![1] as RequestInit).method).toBe('DELETE');
  });
  it('404 is treated as already gone', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x', details: null } }), { status: 404 })));
    const p = props();
    render(View, p);
    await userEvent.click(screen.getByRole('button', { name: 'Hapus' }));
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalled());
  });
});
