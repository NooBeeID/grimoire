import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockCredential, mockGitlabCredential } from '../mocks/credentials';
import { buildRows } from '../types';
import Table from './GitTokensTable.svelte';

beforeEach(() => setLocale('id'));
const rows = () => buildRows([mockCredential(), mockGitlabCredential()]);

describe('GitTokensTable', () => {
  it('saved row shows pill and masked last4; unsaved shows Belum ada and dash', () => {
    render(Table, { props: { rows: buildRows([mockGitlabCredential({ host: 'gitlab.com' })].slice(0, 0)) } });
    expect(screen.getAllByText('Belum ada')).toHaveLength(2);
    expect(screen.getAllByText('–').length).toBe(2);
  });
  it('shows Tersimpan and mono last4', () => {
    render(Table, { props: { rows: rows() } });
    expect(screen.getAllByText('Tersimpan')).toHaveLength(2);
    expect(screen.getByText('…7Qx2').tagName).toBe('CODE');
    expect(screen.getByText('git.noobee.id').tagName).toBe('CODE');
  });
  it('action buttons carry host in aria-label and call back', async () => {
    const onsave = vi.fn();
    const ondelete = vi.fn();
    render(Table, { props: { rows: rows(), onsave, ondelete } });
    await userEvent.click(screen.getByRole('button', { name: 'Ganti token GitLab untuk git.noobee.id' }));
    expect(onsave).toHaveBeenCalledWith(expect.objectContaining({ host: 'git.noobee.id' }));
    await userEvent.click(screen.getByRole('button', { name: 'Hapus token GitHub untuk github.com' }));
    expect(ondelete).toHaveBeenCalled();
  });
  it('no action buttons without callbacks or when canEdit=false', () => {
    const { unmount } = render(Table, { props: { rows: rows() } });
    expect(screen.queryByRole('button')).toBeNull();
    unmount();
    render(Table, { props: { rows: rows(), canEdit: false, onsave: vi.fn(), ondelete: vi.fn() } });
    expect(screen.queryByRole('button')).toBeNull();
  });
  it('error shows retry', async () => {
    const onretry = vi.fn();
    render(Table, { props: { rows: [], error: new ApiError({ status: 500, code: 'INTERNAL', message: 'gagal' }), onretry } });
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalled();
  });
});
