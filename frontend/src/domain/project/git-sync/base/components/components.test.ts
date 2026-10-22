import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { toast, toasts } from '../../../../../shared/stores/toast';
import { mockApiSpecReport, mockConfig, mockKnowledgeReport } from '../mocks';
import { ITEM_STATUSES, RUN_STATUSES } from '../types';
import ConfigSummary from './ConfigSummary.svelte';
import RepoOriginChip from './RepoOriginChip.svelte';
import ReportSummary from './ReportSummary.svelte';
import StatusBadge from './StatusBadge.svelte';
import SyncReportTable from './SyncReportTable.svelte';

beforeEach(() => setLocale('id'));
afterEach(() => {
  vi.unstubAllGlobals();
  toast.clear();
});

describe('StatusBadge', () => {
  const labels: Record<string, string> = {
    success: 'Berhasil',
    partial: 'Berhasil sebagian',
    failed: 'Gagal',
    created: 'Dibuat',
    updated: 'Diperbarui',
    unchanged: 'Tidak berubah',
    skipped: 'Dilewati',
    rejected: 'Ditolak',
  };
  it.each([...RUN_STATUSES, ...ITEM_STATUSES])('%s renders text, never colour only', (s) => {
    render(StatusBadge, { props: { status: s } });
    expect(screen.getByText(labels[s]!)).toBeInTheDocument();
  });
  it('tones', () => {
    const { container } = render(StatusBadge, { props: { status: 'rejected' } });
    expect(container.firstElementChild?.className).toContain('bg-pill-negative-bg');
  });
});

describe('RepoOriginChip', () => {
  it('renders host/repo@branch:folder/', () => {
    render(RepoOriginChip, { props: { host: 'github.com', repoPath: 'noobee/grimoire-docs', branch: 'main', folder: 'docs' } });
    expect(screen.getByText('github.com/noobee/grimoire-docs@main:docs/')).toBeInTheDocument();
  });
  it('empty folder has no colon', () => {
    render(RepoOriginChip, { props: { host: 'github.com', repoPath: 'a/b', branch: 'main' } });
    expect(screen.getByText('github.com/a/b@main')).toBeInTheDocument();
  });
});

describe('ReportSummary', () => {
  it('writes zero and marks rejected > 0 as danger only', () => {
    render(ReportSummary, { props: { summary: { created: 2, updated: 0, unchanged: 0, skipped: 1, rejected: 1 } } });
    const dd = (k: string) => document.querySelector(`dd[data-key="${k}"]`)!;
    expect(dd('updated').textContent).toBe('0');
    expect(dd('rejected').className).toContain('text-danger');
    expect(dd('created').className).not.toContain('text-danger');
  });
  it('rejected 0 is not danger', () => {
    render(ReportSummary, { props: { summary: { created: 0, updated: 0, unchanged: 3, skipped: 0, rejected: 0 } } });
    expect(document.querySelector('dd[data-key="rejected"]')!.className).not.toContain('text-danger');
  });
});

describe('ConfigSummary', () => {
  it('null config -> never synced', () => {
    render(ConfigSummary, { props: { config: null, kind: 'knowledge' } });
    expect(screen.getByText('Belum pernah sync untuk Knowledge project ini.')).toBeInTheDocument();
  });
  it('shows last sync, author, status and origin', () => {
    render(ConfigSummary, { props: { config: mockConfig(), kind: 'knowledge' } });
    expect(screen.getByText(/oleh Rina/)).toBeInTheDocument();
    expect(screen.getByText('Berhasil sebagian')).toBeInTheDocument();
    expect(screen.getByTestId('repo-origin')).toBeInTheDocument();
  });
  it('failed status shows the translated error', () => {
    const config = mockConfig({
      last_status: 'failed',
      last_result: mockKnowledgeReport({ status: 'failed', error_code: 'GIT_ACCESS_DENIED', error_message: 'ditolak' }),
    });
    render(ConfigSummary, { props: { config, kind: 'knowledge' } });
    expect(screen.getByText('Token tidak punya akses ke repo ini.')).toBeInTheDocument();
  });
});

describe('SyncReportTable', () => {
  it('rejected rows come first', () => {
    render(SyncReportTable, { props: { items: mockKnowledgeReport().items, kind: 'knowledge' } });
    const rows = within(screen.getByTestId('report-table')).getAllByRole('row').slice(1);
    expect(rows[0]!.textContent).toContain('docs/legacy/../x.md');
    expect(rows[rows.length - 1]!.textContent).toContain('docs/img/diagram.png');
  });
  it('translates reasons', () => {
    render(SyncReportTable, { props: { items: mockKnowledgeReport().items, kind: 'knowledge' } });
    expect(screen.getAllByText('Hanya file .md, .markdown, dan .txt yang didukung').length).toBeGreaterThan(0);
  });
  it('api_spec shows endpoint count and expands issues', async () => {
    render(SyncReportTable, { props: { items: mockApiSpecReport().items, kind: 'api_spec' } });
    expect(screen.getByText(/14 endpoint/)).toBeInTheDocument();
    const btn = screen.getByRole('button', { name: 'Tampilkan detail' });
    expect(btn).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByText('#/paths/~1charges/get')).toBeNull();
    await userEvent.click(btn);
    expect(btn).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText('#/paths/~1charges/get').tagName).toBe('CODE');
  });
  it('copy reason calls the clipboard and toasts', async () => {
    const writeText = vi.fn(async () => undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    render(SyncReportTable, { props: { items: mockApiSpecReport().items, kind: 'api_spec' } });
    await userEvent.click(screen.getByRole('button', { name: 'Salin alasan' }));
    expect(writeText).toHaveBeenCalledWith('api/openapi/payments.json: Dokumen OpenAPI tidak valid');
    expect(get(toasts).map((x) => x.message)).toContain('Alasan disalin');
  });
  it('truncated adds a note', () => {
    render(SyncReportTable, { props: { items: mockKnowledgeReport().items, kind: 'knowledge', truncated: true, total: 900 } });
    expect(screen.getByText('Menampilkan 4 dari 900 file; ringkasan dihitung dari semua file.')).toBeInTheDocument();
  });
  it('empty list shows the empty sentence', () => {
    render(SyncReportTable, { props: { items: [], kind: 'knowledge' } });
    expect(screen.getByText('Tidak ada file di folder ini.')).toBeInTheDocument();
  });
});
