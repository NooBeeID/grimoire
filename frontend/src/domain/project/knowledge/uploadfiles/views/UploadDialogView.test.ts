import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { xhrFrom } from '../../../../../shared/services/fakeXhr';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { UploadTarget } from '../types';
import View from './UploadDialogView.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => vi.unstubAllGlobals());

const json = (s: number, b: unknown) => new Response(JSON.stringify(b), { status: s });
const props = (over: Record<string, unknown> = {}) => ({ open: true, projectId: 'p1', targetDir: '', onclose: vi.fn(), onuploaded: vi.fn(), ...over });

function pickFolder(paths: string[]) {
  const input = screen.getByTestId('pick-folder') as HTMLInputElement;
  const files = paths.map((p) => {
    const f = new File(['isi'], p.split('/').pop()!);
    Object.defineProperty(f, 'webkitRelativePath', { value: p });
    return f;
  });
  Object.defineProperty(input, 'files', { value: files, configurable: true });
  input.dispatchEvent(new Event('change', { bubbles: true }));
}
function stubUpload(handler: () => Promise<Response> | Response) {
  const h = vi.fn(handler);
  vi.stubGlobal('XMLHttpRequest', xhrFrom(h).Xhr);
  return h;
}
const res = (items: { path: string; status: string; reason?: string }[]) => {
  const summary = { created: 0, updated: 0, unchanged: 0, skipped: 0, rejected: 0 } as Record<string, number>;
  items.forEach((i) => summary[i.status]!++);
  return { summary, items };
};

describe('UploadDialogView', () => {
  it('T05-AC1: folder -> review -> upload -> report, then onuploaded', async () => {
    const f = stubUpload(async () => json(200, res([{ path: 'docs/a.md', status: 'created' }, { path: 'docs/auth/login.md', status: 'created' }, { path: 'docs/auth/flow/x.md', status: 'created' }])));
    const p = props();
    render(View, p);
    pickFolder(['docs/a.md', 'docs/auth/login.md', 'docs/auth/flow/x.md', 'docs/.git/config', 'docs/logo.png']);
    expect(await screen.findByText(/3 file ke Knowledge/)).toBeInTheDocument();
    expect(screen.getByText(/1 file dilewati/)).toBeInTheDocument();
    expect(screen.getByText(/1 file tidak didukung/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Unggah' }));
    expect(await screen.findByText('docs/auth/flow/x.md')).toBeInTheDocument();
    expect(p.onuploaded).toHaveBeenCalled();
    expect(f).toHaveBeenCalledTimes(1);
  });
  it('T05-AC2: per-file reasons are readable and valid files are still created', async () => {
    stubUpload(async () => json(200, res([
      { path: 'ok.md', status: 'created' },
      { path: '../x.md', status: 'rejected', reason: 'path_traversal' },
      { path: 'big.md', status: 'rejected', reason: 'too_large' },
    ])));
    render(View, props());
    pickFolder(['ok.md', '../x.md', 'big.md']);
    await userEvent.click(await screen.findByRole('button', { name: 'Unggah' }));
    expect(await screen.findByText('ok.md')).toBeInTheDocument();
    expect(screen.queryByText('path_traversal')).toBeNull();
    await userEvent.click(screen.getByRole('radio', { name: 'Gagal' }));
    expect(screen.queryByText('ok.md')).toBeNull();
    expect(screen.getByText('big.md')).toBeInTheDocument();
  });
  it('413 on a target rejects all its files and does not call onuploaded', async () => {
    stubUpload(async () => json(413, { error: { code: 'PAYLOAD_TOO_LARGE', message: 'x', details: { reason: 'upload_too_large', max_bytes: 52428800 } } }));
    const p = props();
    render(View, p);
    pickFolder(['a.md']);
    await userEvent.click(await screen.findByRole('button', { name: 'Unggah' }));
    expect((await screen.findAllByRole('alert'))[0]).toHaveTextContent('50 MB');
    expect(p.onuploaded).not.toHaveBeenCalled();
  });
  it('mixed upload: the second target failing does not affect the first', async () => {
    stubUpload(async () => json(200, res([{ path: 'a.md', status: 'created' }])));
    const spec: UploadTarget = {
      key: 'spec',
      labelKey: 'knowledge.upload.target.knowledge',
      accepts: (p) => p.endsWith('.yaml'),
      upload: async () => {
        throw new Error('boom');
      },
    };
    const p = props({ extraTargets: [spec] });
    render(View, p);
    pickFolder(['a.md', 'api.yaml']);
    await userEvent.click(await screen.findByRole('button', { name: 'Unggah' }));
    expect(await screen.findByText('a.md')).toBeInTheDocument();
    expect(screen.getByText('api.yaml')).toBeInTheDocument();
    expect(p.onuploaded).toHaveBeenCalled();
  });
  it('yaml without a registered target is reported as unsupported', async () => {
    render(View, props());
    pickFolder(['api.yaml']);
    expect(await screen.findByText(/1 file tidak didukung/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unggah' })).toBeDisabled();
  });
});

describe('UploadDialogView progress', () => {
  it('shows sending percent, then processing, and cancel aborts the request', async () => {
    const { FakeXhr } = await import('../../../../../shared/services/fakeXhr');
    vi.stubGlobal('XMLHttpRequest', FakeXhr);
    const p = props();
    render(View, p);
    pickFolder(['a.md']);
    await userEvent.click(await screen.findByRole('button', { name: 'Unggah' }));
    const x = FakeXhr.last!;
    x.upload.onprogress?.({ lengthComputable: true, loaded: 25, total: 100 });
    expect(await screen.findByText('Mengirim 25%')).toBeInTheDocument();
    x.upload.onload?.();
    expect(await screen.findByText('Memproses di server...')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Batalkan' }));
    expect(x.aborted).toBe(true);
    expect(await screen.findByText('Upload dibatalkan')).toBeInTheDocument();
    expect(p.onuploaded).not.toHaveBeenCalled();
  });
});
