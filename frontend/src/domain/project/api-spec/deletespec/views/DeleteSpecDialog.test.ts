import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import Dialog from './DeleteSpecDialog.svelte';

const props = () => ({ open: true, projectId: 'p1', spec: { name: 'payment' }, onclose: vi.fn(), ondeleted: vi.fn() });
beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

describe('DeleteSpecDialog', () => {
  it('204 -> ondeleted with the name', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })));
    const p = props();
    render(Dialog, p);
    expect(screen.getByText('Spec "payment" dan indeks endpoint-nya akan dihapus permanen.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Hapus spec' }));
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalledWith('payment'));
  });
  it('404 counts as already deleted', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'x', details: null } }), { status: 404 })));
    const p = props();
    render(Dialog, p);
    await userEvent.click(screen.getByRole('button', { name: 'Hapus spec' }));
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalledWith('payment'));
  });
  it('500 keeps the dialog open with the message and trace id', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'INTERNAL', message: 'gagal server', details: null }, trace_id: 'tr-9' }), { status: 500 })));
    const p = props();
    render(Dialog, p);
    await userEvent.click(screen.getByRole('button', { name: 'Hapus spec' }));
    expect(await screen.findByText('gagal server')).toBeInTheDocument();
    expect(screen.getByText('ID jejak: tr-9')).toBeInTheDocument();
    expect(p.ondeleted).not.toHaveBeenCalled();
  });
});
