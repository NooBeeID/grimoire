import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { SaveTarget } from '../types';
import Dialog from './SaveTokenDialog.svelte';

const TOKEN = 'ghp_e2eValidToken7Qx2';
const saved = { provider: 'github', host: 'github.com', last4: '7Qx2', updated_at: '2026-10-22T07:10:00Z', replaced: false };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const errBody = (code: string, details: unknown = null) => ({ error: { code, message: 'pesan server', details }, trace_id: 'TR-9' });
const props = (over: { target?: SaveTarget } = {}) => ({
  open: true,
  target: { provider: 'github', host: 'github.com', gitlabHosts: ['gitlab.com', 'git.noobee.id'] } as SaveTarget,
  onclose: vi.fn(),
  onsaved: vi.fn(),
  ...over,
});
const type = (v = TOKEN) => userEvent.type(screen.getByLabelText('Token'), v);
const submit = () => userEvent.click(screen.getByRole('button', { name: 'Simpan' }));

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  sessionStorage.clear();
});

describe('SaveTokenDialog', () => {
  it('success: one request, onsaved, close, token never in DOM or storage', async () => {
    const f = vi.fn(async () => json(200, saved));
    vi.stubGlobal('fetch', f);
    const p = props();
    render(Dialog, { props: p });
    await type();
    await submit();
    await vi.waitFor(() => expect(p.onsaved).toHaveBeenCalled());
    expect(f).toHaveBeenCalledTimes(1);
    expect(p.onclose).toHaveBeenCalled();
    expect(document.body.textContent).not.toContain(TOKEN);
    expect(JSON.stringify({ ...localStorage })).not.toContain(TOKEN);
    expect(JSON.stringify({ ...sessionStorage })).not.toContain(TOKEN);
  });
  it('client validation prevents the request', async () => {
    const f = vi.fn();
    vi.stubGlobal('fetch', f);
    render(Dialog, { props: props() });
    await type('short');
    await submit();
    expect(await screen.findByText('Token harus 8–512 karakter')).toBeInTheDocument();
    expect(f).not.toHaveBeenCalled();
  });
  it('TOKEN_INVALID -> field error and the value stays', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(422, errBody('TOKEN_INVALID'))));
    render(Dialog, { props: props() });
    await type();
    await submit();
    expect(await screen.findByText('Token tidak valid atau tidak punya akses')).toBeInTheDocument();
    expect(screen.getByLabelText('Token')).toHaveValue(TOKEN);
  });
  it('HOST_NOT_ALLOWED -> host field error', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(422, errBody('HOST_NOT_ALLOWED'))));
    render(Dialog, { props: props({ target: { provider: 'gitlab', host: null, gitlabHosts: ['git.noobee.id'] } }) });
    await type();
    await submit();
    expect(await screen.findByText(/Host Git tidak diizinkan/)).toBeInTheDocument();
  });
  it('RATE_LIMITED shows the retry seconds', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(429, errBody('RATE_LIMITED', { retry_after_seconds: 60 }))));
    render(Dialog, { props: props() });
    await type();
    await submit();
    expect(await screen.findByText(/coba lagi dalam 60 detik/)).toBeInTheDocument();
  });
  it('GIT_UPSTREAM_ERROR shows the trace id', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(502, errBody('GIT_UPSTREAM_ERROR'))));
    render(Dialog, { props: props() });
    await type();
    await submit();
    expect(await screen.findByText(/TR-9/)).toBeInTheDocument();
  });
  it('GIT_FEATURE_DISABLED disables the form', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => json(503, errBody('GIT_FEATURE_DISABLED'))));
    render(Dialog, { props: props() });
    await type();
    await submit();
    await screen.findByText('Sync Git belum dikonfigurasi oleh admin');
    expect(screen.getByLabelText('Token')).toBeDisabled();
  });
  it('GitHub has no host field; gitlab target does', () => {
    const { unmount } = render(Dialog, { props: props() });
    expect(screen.queryByLabelText('Host')).toBeNull();
    unmount();
    render(Dialog, { props: props({ target: { provider: 'gitlab', host: 'git.noobee.id', gitlabHosts: ['git.noobee.id'] } }) });
    expect(screen.getByLabelText('Host')).toHaveValue('git.noobee.id');
  });
});
