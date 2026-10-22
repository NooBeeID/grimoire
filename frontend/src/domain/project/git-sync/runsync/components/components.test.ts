import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import type { SyncError } from '../types';
import CredentialNotice from './CredentialNotice.svelte';
import SyncErrorBlock from './SyncErrorBlock.svelte';
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
    expect(screen.getByRole('link', { name: 'Atur Token Git' })).toHaveAttribute('href', '/profile#token-git');
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
    render(SyncErrorBlock, { props: { error: err({ code: 'CREDENTIAL_MISSING', host: 'github.com', settingsPath: '/profile#token-git' }) } });
    expect(screen.getByRole('link', { name: 'Atur Token Git' })).toHaveAttribute('href', '/profile#token-git');
  });
  it('shows the trace id and a retry button for SYNC_IN_PROGRESS', async () => {
    const onretry = vi.fn();
    render(SyncErrorBlock, { props: { error: err({ code: 'SYNC_IN_PROGRESS', traceId: 'TR-5' }), onretry } });
    expect(screen.getByText(/TR-5/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalled();
  });
});
