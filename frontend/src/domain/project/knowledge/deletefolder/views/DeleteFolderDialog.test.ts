import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import View from './DeleteFolderDialog.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const props = (over: Record<string, unknown> = {}) => ({ open: true, projectId: 'p1', path: 'docs', fileCount: 4, ondeleted: vi.fn(), onclose: vi.fn(), ...over });

describe('DeleteFolderDialog', () => {
  it('shows the file count and requires the acknowledgement', async () => {
    render(View, props());
    expect(screen.getByText('4 file di bawah folder ini akan dihapus permanen.')).toBeInTheDocument();
    const btn = screen.getByRole('button', { name: 'Hapus folder' });
    expect(btn).toBeDisabled();
    await userEvent.click(screen.getByRole('checkbox'));
    expect(btn).toBeEnabled();
  });
  it('deletes and reports the deleted count', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ deleted: 4 }), { status: 200 })));
    const p = props();
    render(View, p);
    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(screen.getByRole('button', { name: 'Hapus folder' }));
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalledWith('docs', 4));
  });
  it('404 reports zero deleted', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x', details: null } }), { status: 404 })));
    const p = props();
    render(View, p);
    await userEvent.click(screen.getByRole('checkbox'));
    await userEvent.click(screen.getByRole('button', { name: 'Hapus folder' }));
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalledWith('docs', 0));
  });
  it('an empty path can never be confirmed', async () => {
    render(View, props({ path: '' }));
    await userEvent.click(screen.getByRole('checkbox'));
    expect(screen.getByRole('button', { name: 'Hapus folder' })).toBeDisabled();
  });
});
