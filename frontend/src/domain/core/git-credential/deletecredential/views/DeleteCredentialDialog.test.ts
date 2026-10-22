import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import Dialog from './DeleteCredentialDialog.svelte';

const props = () => ({
  open: true,
  target: { provider: 'gitlab' as const, host: 'git.noobee.id', last4: '7Qx2' },
  onclose: vi.fn(),
  ondeleted: vi.fn(),
});
const errBody = (code: string) => ({ error: { code, message: 'pesan server', details: null }, trace_id: 'TR-3' });
const confirm = () => userEvent.click(screen.getByRole('button', { name: 'Hapus' }));

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

describe('DeleteCredentialDialog', () => {
  it('names provider, host and last4', () => {
    render(Dialog, { props: props() });
    expect(screen.getByText('git.noobee.id')).toBeInTheDocument();
    expect(screen.getByText('…7Qx2')).toBeInTheDocument();
  });
  it('confirm: service, close, ondeleted, toast', async () => {
    const f = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, { props: p });
    await confirm();
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalled());
    expect(f).toHaveBeenCalledTimes(1);
    expect(p.onclose).toHaveBeenCalled();
    expect(get(toasts).map((x) => x.message)).toContain('Token GitLab dihapus');
  });
  it('404 -> info toast and ondeleted', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(errBody('NOT_FOUND')), { status: 404 })));
    const p = props();
    render(Dialog, { props: p });
    await confirm();
    await vi.waitFor(() => expect(p.ondeleted).toHaveBeenCalled());
    expect(get(toasts).map((x) => x.message)).toContain('Token sudah tidak ada');
  });
  it('500 stays open with message and trace id', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(errBody('INTERNAL')), { status: 500 })));
    const p = props();
    render(Dialog, { props: p });
    await confirm();
    expect(await screen.findByText(/pesan server/)).toBeInTheDocument();
    expect(screen.getByText(/TR-3/)).toBeInTheDocument();
    expect(p.onclose).not.toHaveBeenCalled();
  });
  it('Batal does not call the service', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, { props: p });
    await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
    expect(f).not.toHaveBeenCalled();
    expect(p.onclose).toHaveBeenCalled();
  });
});
