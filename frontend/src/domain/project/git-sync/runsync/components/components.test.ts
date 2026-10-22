import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { SyncError } from '../types';
import CredentialNotice from './CredentialNotice.svelte';
import CloseRunningConfirm from './CloseRunningConfirm.svelte';
import InProgressNotice from './InProgressNotice.svelte';
import SyncErrorBlock from './SyncErrorBlock.svelte';
import SyncProgress from './SyncProgress.svelte';
import SyncForm from './SyncFormHarness.test.svelte';

beforeEach(() => setLocale('id'));

const form = (kind: 'knowledge' | 'api_spec') => ({ kind });

describe('SyncForm', () => {
  it('a tree URL fills Branch and Folder when untouched', async () => {
    render(SyncForm, { props: form('knowledge') });
    await userEvent.type(screen.getByLabelText('URL repo atau folder'), 'https://github.com/noobee/grimoire-docs/tree/main/docs');
    expect(screen.getByLabelText('Branch')).toHaveValue('main');
    expect(screen.getByLabelText('Folder di repo')).toHaveValue('docs');
  });
  it('does not overwrite a touched field', async () => {
    render(SyncForm, { props: form('knowledge') });
    await userEvent.type(screen.getByLabelText('Branch'), 'dev');
    await userEvent.type(screen.getByLabelText('URL repo atau folder'), 'https://github.com/noobee/grimoire-docs/tree/main/docs');
    expect(screen.getByLabelText('Branch')).toHaveValue('dev');
    expect(screen.getByLabelText('Folder di repo')).toHaveValue('docs');
  });
  it('Simpan di folder only for knowledge', () => {
    const { unmount } = render(SyncForm, { props: form('knowledge') });
    expect(screen.getByLabelText('Simpan di folder')).toBeInTheDocument();
    unmount();
    render(SyncForm, { props: form('api_spec') });
    expect(screen.queryByLabelText('Simpan di folder')).toBeNull();
  });
  it('shows a field error with aria-invalid', () => {
    render(SyncForm, { props: { ...form('api_spec'), errors: { branch: 'isi Branch' } } });
    expect(screen.getByLabelText('Branch')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByText('isi Branch')).toBeInTheDocument();
  });
});

describe('CredentialNotice', () => {
  it('links to the settings anchor', () => {
    render(CredentialNotice, { props: { state: 'missing', host: 'github.com' } });
    expect(screen.getByText('Anda belum menyimpan token Git untuk github.com.')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Atur Token Git' })).toHaveAttribute('href', '/settings#token-git');
  });
  it('unreadable asks to save again', () => {
    render(CredentialNotice, { props: { state: 'unreadable', host: 'github.com' } });
    expect(screen.getByText(/simpan ulang token/)).toBeInTheDocument();
  });
  it('disabled has no link and a mono key', () => {
    render(CredentialNotice, { props: { state: 'disabled' } });
    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('GIT_CREDENTIAL_KEY').tagName).toBe('CODE');
  });
});

describe('SyncErrorBlock', () => {
  const err = (e: Partial<SyncError> & { code: string }): SyncError => ({ message: 'pesan server', status: 422, ...e });
  it.each([
    [err({ code: 'SYNC_IN_PROGRESS' }), 'Sync lain untuk project ini sedang berjalan. Coba lagi sebentar lagi.'],
    [err({ code: 'SYNC_LIMIT_EXCEEDED', limit: { files: 742, max_files: 500, known_bytes: 1, max_total_bytes: 2 } }), '742 file, melebihi batas 500 per sync; persempit folder.'],
    [err({ code: 'GIT_NOT_FOUND', field: 'folder', folder: 'tidak-ada' }), 'Folder tidak-ada tidak ditemukan di branch ini.'],
    [err({ code: 'HOST_NOT_ALLOWED' }), 'Host Git tidak diizinkan; minta admin menambahkannya ke GIT_SYNC_ALLOWED_HOSTS.'],
    [err({ code: 'TIMEOUT' }), 'Sync melebihi batas waktu. Hasil sebagian mungkin sudah tersimpan; buka lagi untuk melihat status terakhir.'],
    [err({ code: 'INVALID_REPO_URL' }), 'pesan server'],
  ])('%j', (e, text) => {
    render(SyncErrorBlock, { props: { error: e } });
    expect(screen.getByText(text)).toBeInTheDocument();
  });
  it('RATE_LIMITED shows the countdown', () => {
    render(SyncErrorBlock, { props: { error: err({ code: 'RATE_LIMITED', retryAfter: 60 }), seconds: 42 } });
    expect(screen.getByText(/coba lagi dalam 42 detik/)).toBeInTheDocument();
  });
  it('CREDENTIAL_MISSING renders the notice with the settings link', () => {
    render(SyncErrorBlock, { props: { error: err({ code: 'CREDENTIAL_MISSING', host: 'github.com', settingsPath: '/settings#token-git' }) } });
    expect(screen.getByRole('link', { name: 'Atur Token Git' })).toHaveAttribute('href', '/settings#token-git');
  });
  it('shows the trace id and a retry button for SYNC_IN_PROGRESS', async () => {
    const onretry = vi.fn();
    render(SyncErrorBlock, { props: { error: err({ code: 'SYNC_IN_PROGRESS', traceId: 'TR-5' }), onretry } });
    expect(screen.getByText(/TR-5/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalled();
  });
});

describe('SyncErrorBlock T17', () => {
  const rl: SyncError = { code: 'RATE_LIMITED', message: 'm', status: 200, retryAfter: 60 };
  it('RATE_LIMITED retry is disabled during the countdown and enabled at 0', async () => {
    const onretry = vi.fn();
    const { unmount } = render(SyncErrorBlock, { props: { error: rl, seconds: 12, onretry } });
    expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeDisabled();
    unmount();
    render(SyncErrorBlock, { props: { error: rl, seconds: 0, onretry } });
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalled();
  });
  it('lost mode shows a warning and Muat ulang status instead of the danger message', async () => {
    const onreloadstatus = vi.fn();
    render(SyncErrorBlock, { props: { error: { code: 'NETWORK', message: '', status: 0 }, lost: true, onreloadstatus } });
    expect(screen.getByText(/Koneksi ke server terputus/).className).not.toContain('text-danger');
    await userEvent.click(screen.getByRole('button', { name: 'Muat ulang status' }));
    expect(onreloadstatus).toHaveBeenCalled();
  });
});

describe('SyncProgress', () => {
  it('listing: indeterminate bar, dash on the right, no fill', () => {
    render(SyncProgress, { props: { progress: { phase: 'listing' }, now: 0 } });
    expect(screen.getByText('Mengambil daftar file…')).toBeInTheDocument();
    expect(screen.getByTestId('sync-progress-percent')).toHaveTextContent('–');
    const bar = screen.getByRole('progressbar');
    expect(bar).not.toHaveAttribute('aria-valuenow');
    expect(document.querySelector('.bg-accent')).toBeNull();
  });
  it('fetching 7/42: counts, percent, truncated mono path with full title', () => {
    const path = 'docs/very/long/nested/folder/structure/auth/login-flow.md';
    render(SyncProgress, { props: { progress: { phase: 'fetching', done: 7, total: 42, path }, now: 0 } });
    expect(screen.getByText('7 dari 42')).toBeInTheDocument();
    expect(screen.getByText('16%')).toBeInTheDocument();
    const bar = screen.getByRole('progressbar');
    expect(bar).toHaveAttribute('aria-valuenow', '7');
    expect(bar).toHaveAttribute('aria-valuemax', '42');
    expect(bar).toHaveAttribute('aria-valuetext', '7 dari 42');
    const p = screen.getByTestId('sync-progress-path');
    expect(p.className).toContain('font-mono'); // taste-skill-ignore: file path is literal code
    expect(p).toHaveAttribute('title', path);
    expect(p.textContent!.endsWith('login-flow.md')).toBe(true);
    expect(document.querySelector('.bg-accent')).not.toBeNull();
  });
  it('waiting: countdown from now, neutral fill, clock icon, live region only changes per 5 s', async () => {
    const progress = { phase: 'waiting' as const, done: 7, total: 42, path: 'a.md', retryAfter: 38, until: 38_000 };
    const { rerender } = render(SyncProgress, { props: { progress, now: 3000 } });
    expect(screen.getByTestId('sync-waiting')).toHaveTextContent('lanjut dalam 35 detik');
    const fill = screen.getByTestId('sync-progress-fill');
    expect(fill.className).toContain('bg-border');
    expect(fill.className).not.toContain('bg-danger');
    expect(screen.getByTestId('sync-waiting').className).not.toContain('text-danger');
    expect(screen.getByTestId('sync-waiting-icon')).toBeInTheDocument();
    const live = () => screen.getByTestId('sync-progress-live').textContent;
    const first = live();
    await rerender({ progress, now: 4000 });
    expect(live()).toBe(first);
    await rerender({ progress, now: 8000 });
    expect(live()).not.toBe(first);
  });
  it('waiting before start (total null) stays indeterminate', () => {
    render(SyncProgress, { props: { progress: { phase: 'waiting', done: 0, total: null, path: '', retryAfter: 5, until: 5000 }, now: 0 } });
    expect(screen.getByText('Mengambil daftar file…')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).not.toHaveAttribute('aria-valuenow');
  });
  it('saving: 100% and Menyimpan label', () => {
    render(SyncProgress, { props: { progress: { phase: 'saving', total: 42 }, now: 0 } });
    expect(screen.getByTestId('sync-progress-count')).toHaveTextContent('Menyimpan…');
    expect(screen.getByText('100%')).toBeInTheDocument();
    expect(screen.getByTestId('sync-progress-fill').getAttribute('style')).toContain('100%');
  });
});

describe('InProgressNotice', () => {
  it('shows title, reload calls onreload, loading disables, role status', async () => {
    const onreload = vi.fn();
    const { rerender } = render(InProgressNotice, { props: { loading: false, onreload } });
    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.getByText('Sync sedang berjalan di server')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Muat ulang status' }));
    expect(onreload).toHaveBeenCalled();
    await rerender({ loading: true, onreload });
    expect(screen.getByRole('button', { name: 'Muat ulang status' })).toBeDisabled();
  });
});

describe('CloseRunningConfirm', () => {
  it('initial focus on Tetap di sini; Esc stays; Tutup confirms', async () => {
    const onstay = vi.fn();
    const onconfirm = vi.fn();
    render(CloseRunningConfirm, { props: { open: true, onconfirm, onstay } });
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Tetap di sini' })).toHaveFocus());
    await userEvent.click(screen.getAllByRole('button', { name: 'Tutup' }).at(-1)!);
    expect(onconfirm).toHaveBeenCalled();
    document.querySelector('dialog')!.dispatchEvent(new Event('close'));
    await vi.waitFor(() => expect(onstay).toHaveBeenCalled());
  });
});
