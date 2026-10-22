import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import { mockApiSpecReport, mockConfig, mockKnowledgeReport } from '../../base';
import Dialog from './GitSyncDialog.svelte';

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const errBody = (code: string, details: unknown = null, message = 'pesan server') => ({ error: { code, message, details }, trace_id: 'TR-2' });
const cfgResp = (over: Record<string, unknown> = {}) => ({
  kind: 'knowledge',
  feature_enabled: true,
  has_credential: true,
  credential_unreadable: false,
  config: mockConfig(),
  ...over,
});
const runResp = (report = mockKnowledgeReport(), config = mockConfig({ last_result: null })) => ({ report, config });

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response>;
function stub(handlers: { get?: Handler; run?: Handler }) {
  const f = vi.fn(async (url: string, init?: RequestInit) => {
    if (url.includes('/git-sync/run')) return handlers.run!(url, init);
    return handlers.get ? handlers.get(url, init) : json(200, cfgResp());
  });
  vi.stubGlobal('fetch', f);
  return f;
}
const runCalls = (f: ReturnType<typeof vi.fn>) => f.mock.calls.filter((c) => String(c[0]).includes('/run'));
const props = (over: Record<string, unknown> = {}) => ({ open: true, projectId: 'p1', kind: 'knowledge' as const, onclose: vi.fn(), onsynced: vi.fn(), ...over });
const btn = (name: string) => screen.getByRole('button', { name });

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

describe('GitSyncDialog', () => {
  it('open -> getConfig with kind; form is filled and summary shown', async () => {
    const f = stub({});
    render(Dialog, { props: props() });
    await screen.findByDisplayValue('https://github.com/noobee/grimoire-docs/tree/main/docs');
    expect(String(f.mock.calls[0]![0])).toBe('/api/projects/p1/git-sync?kind=knowledge');
    expect(screen.getByLabelText('Branch')).toHaveValue('main');
    expect(screen.getByLabelText('Folder di repo')).toHaveValue('docs');
    expect(screen.getByText(/oleh Rina/)).toBeInTheDocument();
    expect(screen.getByText('File bernama sama akan ditimpa. File yang tidak ada di repo tidak dihapus.')).toBeInTheDocument();
  });

  it('config null: never-synced text and Sync is ENABLED even though has_credential=false (F5)', async () => {
    stub({ get: () => json(200, cfgResp({ config: null, has_credential: false })) });
    render(Dialog, { props: props() });
    await screen.findByText('Belum pernah sync untuk Knowledge project ini.');
    await userEvent.type(screen.getByLabelText('URL repo atau folder'), 'https://github.com/noobee/grimoire-docs');
    expect(btn('Sync')).toBeEnabled();
    expect(btn('Sync ulang')).toBeDisabled();
  });

  it('config + no credential + same host: Sync and Sync ulang locked, notice shown; another host unlocks (F6)', async () => {
    stub({ get: () => json(200, cfgResp({ has_credential: false })) });
    render(Dialog, { props: props() });
    await screen.findByText('Anda belum menyimpan token Git untuk github.com.');
    expect(btn('Sync')).toBeDisabled();
    expect(btn('Sync ulang')).toBeDisabled();
    const url = screen.getByLabelText('URL repo atau folder');
    await userEvent.clear(url);
    await userEvent.type(url, 'https://git.noobee.id/grp/repo');
    expect(btn('Sync')).toBeEnabled();
    expect(screen.queryByTestId('credential-notice')).toBeNull();
  });

  it('credential_unreadable asks to save again', async () => {
    stub({ get: () => json(200, cfgResp({ credential_unreadable: true })) });
    render(Dialog, { props: props() });
    expect(await screen.findByText(/simpan ulang token/)).toBeInTheDocument();
    expect(btn('Sync')).toBeDisabled();
  });

  it('feature disabled: form locked, only Tutup', async () => {
    stub({ get: () => json(200, cfgResp({ feature_enabled: false, config: null })) });
    render(Dialog, { props: props() });
    expect((await screen.findByText('GIT_CREDENTIAL_KEY')).tagName).toBe('CODE');
    expect(screen.getByLabelText('URL repo atau folder')).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Sync' })).toBeNull();
    expect(screen.getAllByRole('button', { name: 'Tutup' }).length).toBeGreaterThan(0);
  });

  it('client validation: http URL disables Sync with a message; userinfo sends nothing', async () => {
    const f = stub({ get: () => json(200, cfgResp({ config: null })) });
    render(Dialog, { props: props() });
    await screen.findByText(/Belum pernah sync/);
    const url = screen.getByLabelText('URL repo atau folder');
    await userEvent.type(url, 'http://github.com/a/b');
    expect(screen.getByText('Hanya URL https yang didukung')).toBeInTheDocument();
    expect(btn('Sync')).toBeDisabled();
    await userEvent.clear(url);
    await userEvent.type(url, 'https://ghp_x@github.com/a/b');
    expect(screen.getByText('Jangan sertakan nama pengguna atau token di URL')).toBeInTheDocument();
    expect(runCalls(f)).toHaveLength(0);
  });

  it('run: running state, then partial result; onsynced exactly once; summary equals rows', async () => {
    let resolve!: (r: Response) => void;
    stub({ run: () => new Promise<Response>((r) => (resolve = r)) });
    const p = props();
    render(Dialog, { props: p });
    await screen.findByDisplayValue(/grimoire-docs/);
    await userEvent.click(btn('Sync'));
    expect(await screen.findByText('Menarik dari GitHub…')).toBeInTheDocument();
    expect(screen.getByLabelText('URL repo atau folder')).toBeDisabled();
    expect(screen.getByText('Sync berjalan di server; bisa sampai 2 menit.')).toBeInTheDocument();
    resolve(json(200, runResp()));
    expect(await screen.findByText('docs/legacy/../x.md')).toBeInTheDocument();
    expect(p.onsynced).toHaveBeenCalledTimes(1);
    const rows = screen.getAllByRole('row').length - 1;
    expect(rows).toBe(4);
    expect(screen.getByTestId('report-summary').textContent).toContain('2');
    expect(screen.getByText('Berhasil sebagian')).toBeInTheDocument();
  });

  it('Sync lagi returns to the filled form', async () => {
    stub({ run: () => json(200, runResp()) });
    render(Dialog, { props: props() });
    await screen.findByDisplayValue(/grimoire-docs/);
    await userEvent.click(btn('Sync'));
    await screen.findByText('docs/legacy/../x.md');
    await userEvent.click(btn('Sync lagi'));
    expect(screen.getByLabelText('Branch')).toHaveValue('main');
  });

  it('Sync ulang sends the saved config, not the edited form', async () => {
    const f = stub({ run: () => json(200, runResp()) });
    render(Dialog, { props: props() });
    const url = await screen.findByDisplayValue(/grimoire-docs/);
    await userEvent.clear(url);
    await userEvent.type(url, 'https://github.com/other/repo');
    await userEvent.click(btn('Sync ulang'));
    await vi.waitFor(() => expect(runCalls(f)).toHaveLength(1));
    const body = JSON.parse(runCalls(f)[0]![1].body as string);
    expect(body).toMatchObject({ url: 'https://github.com/noobee/grimoire-docs/tree/main/docs', branch: 'main', folder: 'docs' });
  });

  it('api_spec kind shows no target prefix and no target_prefix in the body', async () => {
    const f = stub({ get: () => json(200, cfgResp({ kind: 'api_spec', config: null })), run: () => json(200, runResp(mockApiSpecReport())) });
    render(Dialog, { props: props({ kind: 'api_spec' }) });
    await screen.findByText(/Belum pernah sync untuk API Spec/);
    expect(screen.queryByLabelText('Simpan di folder')).toBeNull();
    await userEvent.type(screen.getByLabelText('URL repo atau folder'), 'https://github.com/noobee/grimoire-docs');
    await userEvent.type(screen.getByLabelText('Branch'), 'main');
    await userEvent.click(btn('Sync'));
    await screen.findByText('api/openapi/billing.yaml');
    expect(JSON.parse(runCalls(f)[0]![1].body as string)).not.toHaveProperty('target_prefix');
  });

  it('CREDENTIAL_MISSING from the server shows the notice and link', async () => {
    stub({
      get: () => json(200, cfgResp({ config: null, has_credential: false })),
      run: () => json(422, errBody('CREDENTIAL_MISSING', { host: 'github.com', settings_path: '/profile#token-git' })),
    });
    render(Dialog, { props: props() });
    await screen.findByText(/Belum pernah sync/);
    await userEvent.type(screen.getByLabelText('URL repo atau folder'), 'https://github.com/noobee/grimoire-docs');
    await userEvent.click(btn('Sync'));
    expect(await screen.findByRole('link', { name: 'Atur Token Git' })).toHaveAttribute('href', '/profile#token-git');
  });

  it('INVALID_REPO_URL field=branch marks the Branch field', async () => {
    stub({ run: () => json(400, errBody('INVALID_REPO_URL', { field: 'branch' }, 'isi Branch')) });
    render(Dialog, { props: props() });
    await screen.findByDisplayValue(/grimoire-docs/);
    await userEvent.click(btn('Sync'));
    expect(await screen.findByText('isi Branch')).toBeInTheDocument();
    expect(screen.getByLabelText('Branch')).toHaveAttribute('aria-invalid', 'true');
  });

  it('SYNC_IN_PROGRESS shows message and retry', async () => {
    stub({ run: () => json(409, errBody('SYNC_IN_PROGRESS')) });
    render(Dialog, { props: props() });
    await screen.findByDisplayValue(/grimoire-docs/);
    await userEvent.click(btn('Sync'));
    expect(await screen.findByText(/sedang berjalan/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeInTheDocument();
  });

  it('Batal while running aborts, closes, toasts, and does not call onsynced', async () => {
    stub({
      run: (_u, init) => new Promise<Response>((_, rej) => init!.signal!.addEventListener('abort', () => rej(new DOMException('x', 'AbortError')))),
    });
    const p = props();
    render(Dialog, { props: p });
    await screen.findByDisplayValue(/grimoire-docs/);
    await userEvent.click(btn('Sync'));
    await screen.findByText('Menarik dari GitHub…');
    await userEvent.click(btn('Batal'));
    expect(p.onclose).toHaveBeenCalled();
    expect(p.onsynced).not.toHaveBeenCalled();
    expect(get(toasts).map((x) => x.message)).toContain('Sync tetap diselesaikan server; buka lagi untuk melihat hasilnya.');
  });

  it('config load failure keeps the form usable with a retry', async () => {
    stub({ get: () => json(500, errBody('INTERNAL')) });
    render(Dialog, { props: props() });
    expect(await screen.findByText('pesan server')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeInTheDocument();
    expect(screen.getByLabelText('URL repo atau folder')).toBeEnabled();
  });

  it('NOT_FOUND on run closes the dialog and reports the project gone', async () => {
    stub({ run: () => json(404, errBody('NOT_FOUND')) });
    const p = { ...props(), onprojectgone: vi.fn() };
    render(Dialog, { props: p });
    await screen.findByDisplayValue(/grimoire-docs/);
    await userEvent.click(btn('Sync'));
    await vi.waitFor(() => expect(p.onprojectgone).toHaveBeenCalled());
    expect(p.onclose).toHaveBeenCalled();
  });

  it('never renders token material', async () => {
    stub({});
    render(Dialog, { props: props() });
    await screen.findByDisplayValue(/grimoire-docs/);
    expect(document.querySelector('input[type="password"]')).toBeNull();
  });
});
