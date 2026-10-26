import { render, screen, within } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { mockKeys, mockTopFiles, mockUnreadFiles, mockZeroQueries } from '../mocks/usage';
import type { SectionState } from '../stores/usage.store';
import InsightTabs from './InsightTabs.svelte';
import KeyUsageTable from './KeyUsageTable.svelte';
import TopFilesTable from './TopFilesTable.svelte';
import UnreadFilesTable from './UnreadFilesTable.svelte';
import ZeroQueriesTable from './ZeroQueriesTable.svelte';

beforeEach(() => setLocale('id'));
const st = <T,>(status: SectionState<T>['status'], items: T[] = [], extra: Partial<SectionState<T>> = {}): SectionState<T> => ({
  status,
  items,
  truncated: false,
  error: null,
  ...extra,
});

describe('usage tables', () => {
  it('loading: table is aria-busy', () => {
    render(TopFilesTable, { state: st<never>('loading'), onretry: () => {} });
    expect(screen.getByRole('table')).toHaveAttribute('aria-busy', 'true');
  });
  it('empty: shows empty text per table', () => {
    render(TopFilesTable, { state: st<never>('empty'), onretry: () => {} });
    expect(screen.getByText('Belum ada dokumen yang dibaca pada rentang ini')).toBeInTheDocument();
  });
  it('error per section: ErrorState and retry', async () => {
    const onretry = vi.fn();
    render(ZeroQueriesTable, { state: st('error', [], { error: 'unavailable' }), onretry });
    expect(screen.getByText('Bagian ini gagal dimuat.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalledTimes(1);
  });
  it('needs project: shows Pilih project', () => {
    render(UnreadFilesTable, { state: st<never>('needs_project'), onretry: () => {} });
    expect(screen.getByText('Pilih project')).toBeInTheDocument();
    expect(screen.queryByRole('table')).toBeNull();
  });
  it('renders path in monospace with title attr', () => {
    const { container } = render(TopFilesTable, { state: st('ready', mockTopFiles), onretry: () => {} });
    const el = container.querySelector(`.font-mono[title="${mockTopFiles[0]!.path}"]`);
    expect(el).not.toBeNull();
  });
  it('unread truncated note', () => {
    render(UnreadFilesTable, { state: st('ready', mockUnreadFiles, { truncated: true }), onretry: () => {} });
    expect(screen.getByText(`Menampilkan ${mockUnreadFiles.length} pertama.`)).toBeInTheDocument();
  });
  it('renders zero queries and key usage rows', () => {
    render(ZeroQueriesTable, { state: st('ready', mockZeroQueries), onretry: () => {} });
    expect(screen.getByText('idempotency key retry')).toBeInTheDocument();
    render(KeyUsageTable, { state: st('ready', mockKeys), onretry: () => {} });
    const row = screen.getByText('ci-review-agent').closest('tr')!;
    expect(within(row).getByText('read_file')).toBeInTheDocument();
  });
});

describe('InsightTabs', () => {
  it('ArrowRight moves to the next tab', async () => {
    const onchange = vi.fn();
    render(InsightTabs, { value: 'top', onchange });
    screen.getByRole('tab', { name: 'Paling sering dibaca' }).focus();
    await userEvent.keyboard('{ArrowRight}');
    expect(onchange).toHaveBeenCalledWith('unread');
  });
  it('marks active tab', () => {
    render(InsightTabs, { value: 'search', onchange: () => {} });
    expect(screen.getByRole('tab', { name: 'Pencarian' })).toHaveAttribute('aria-selected', 'true');
  });
});
