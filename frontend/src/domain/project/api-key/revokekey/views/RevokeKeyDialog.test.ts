import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import { mockRevoked } from '../mocks/revoked';
import Dialog from './RevokeKeyDialog.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const errBody = (code: string, details: unknown = null) => ({ error: { code, message: 'pesan server', details }, trace_id: 'TR-7' });
const props = () => ({
  open: true,
  projectId: 'p1',
  target: { id: 'k1', name: 'ci-pipeline', prefix: 'a1b2c3d4' },
  onclose: vi.fn(),
  onrevoked: vi.fn(),
  onstale: vi.fn(),
  onprojectgone: vi.fn(),
});
const confirm = () => userEvent.click(screen.getByRole('button', { name: 'Cabut key' }));

describe('RevokeKeyDialog', () => {
  it('shows name and prefix', () => {
    render(Dialog, { props: props() });
    expect(screen.getByText(/ci-pipeline/)).toBeInTheDocument();
    expect(screen.getByText('a1b2c3d4')).toBeInTheDocument();
  });
  it('success: onrevoked, toast, close', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(200, mockRevoked())));
    const p = props();
    render(Dialog, { props: p });
    await confirm();
    await vi.waitFor(() => expect(p.onrevoked).toHaveBeenCalled());
    expect(p.onclose).toHaveBeenCalled();
    expect(get(toasts).map((x) => x.message)).toContain('Key "ci-pipeline" dicabut');
  });
  it.each([
    [409, 'CONFLICT', 'ALREADY_REVOKED', 'Key ini sudah dicabut. Daftar diperbarui.'],
    [404, 'NOT_FOUND', 'KEY_NOT_FOUND', 'Key tidak ditemukan. Daftar diperbarui.'],
  ])('%i %s -> toast + onstale', async (status, code, reason, msg) => {
    vi.stubGlobal('fetch', vi.fn(async () => json(status, errBody(code, { reason }))));
    const p = props();
    render(Dialog, { props: p });
    await confirm();
    await vi.waitFor(() => expect(p.onstale).toHaveBeenCalled());
    expect(get(toasts).map((x) => x.message)).toContain(msg);
  });
  it('404 without reason -> onprojectgone', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(404, errBody('NOT_FOUND'))));
    const p = props();
    render(Dialog, { props: p });
    await confirm();
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
  });
  it('500 stays open with message and can retry', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(500, errBody('INTERNAL'))));
    const p = props();
    render(Dialog, { props: p });
    await confirm();
    expect(await screen.findByText('pesan server')).toBeInTheDocument();
    expect(p.onclose).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Cabut key' })).toBeEnabled();
  });
  it('Batal does not fetch', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, { props: p });
    await userEvent.click(screen.getByRole('button', { name: 'Batal' }));
    expect(f).not.toHaveBeenCalled();
    expect(p.onclose).toHaveBeenCalled();
  });
});
