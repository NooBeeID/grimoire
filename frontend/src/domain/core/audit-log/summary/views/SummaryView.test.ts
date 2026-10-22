import { cleanup, render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../../../../../shared/stores/i18n';
import { created } from '../../../../../test-stubs/chartjs';
import {
  mockProjectOptions,
  mockSummary24hProject,
  mockSummary7d,
  mockSummaryEmpty,
  mockSummaryQualityUnavailable,
  PROJECT_IDS,
} from '../mocks/summary';
import SummaryView from './SummaryView.svelte';

vi.mock('chart.js', async () => (await import('../../../../../test-stubs/chartjs')).chartJsMock());

let summary: unknown;
let summaryStatus = 200;
let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  setLocale('id');
  created.length = 0;
  summary = mockSummary7d;
  summaryStatus = 200;
  fetchMock = vi.fn(async (url: string) => {
    if (url.startsWith('/api/projects')) return new Response(JSON.stringify(mockProjectOptions), { status: 200 });
    if (summaryStatus !== 200)
      return new Response(JSON.stringify({ error: { code: 'INTERNAL', message: 'gagal' }, trace_id: 'tr-1' }), { status: summaryStatus });
    return new Response(JSON.stringify(summary), { status: 200 });
  });
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

const mount = (qs = '', onquerychange = vi.fn()) => ({
  onquerychange,
  ...render(SummaryView, { query: new URLSearchParams(qs), onquerychange }),
});

describe('SummaryView', () => {
  it('shows figures and five charts without a project', async () => {
    mount();
    expect(await screen.findByText('1.234')).toBeInTheDocument();
    expect(screen.getByText('2,5%')).toBeInTheDocument();
    expect(screen.getByText('31 error')).toBeInTheDocument();
    await waitFor(() => expect(created).toHaveLength(5));
    expect(screen.getByText('Pilih project untuk melihat kualitas pencarian')).toBeInTheDocument();
    expect(fetchMock.mock.calls.find((c) => String(c[0]).startsWith('/api/audit-log/summary'))![0]).toBe('/api/audit-log/summary?range=7d');
  });

  it('each chart exposes an sr-only data table (K8)', async () => {
    mount();
    await screen.findByText('1.234');
    const tables = document.querySelectorAll('table.sr-only');
    expect(tables.length).toBe(5);
    expect(screen.getByRole('table', { name: 'Trafik' })).toBeInTheDocument();
  });

  it('range change is reported through onquerychange', async () => {
    const { onquerychange } = mount();
    await screen.findByText('1.234');
    await userEvent.click(screen.getByLabelText('24 jam'));
    expect(onquerychange).toHaveBeenCalledTimes(1);
    expect(onquerychange.mock.calls[0]![0].toString()).toBe('range=24h');
  });

  it('with a project: quality block shows ratios and ranks', async () => {
    summary = mockSummary24hProject;
    mount(`range=24h&project=${PROJECT_IDS.payments}`);
    expect(await screen.findByText('Kualitas pencarian')).toBeInTheDocument();
    expect(await screen.findByText('4/59')).toBeInTheDocument();
    expect(screen.getByText('6,8%')).toBeInTheDocument();
    expect(screen.getByText('Perkiraan tren dari jeda 120 detik, bukan angka mutlak.')).toBeInTheDocument();
  });

  it('quality unavailable keeps other charts and retries on its own', async () => {
    summary = mockSummaryQualityUnavailable;
    mount(`range=24h&project=${PROJECT_IDS.payments}`);
    expect(await screen.findByText('Kualitas pencarian tidak dapat dimuat.')).toBeInTheDocument();
    expect(screen.getByText('65')).toBeInTheDocument();
    summary = mockSummary24hProject;
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(await screen.findByText('4/59')).toBeInTheDocument();
  });

  it('empty project: zero figures and dash, no searches', async () => {
    summary = mockSummaryEmpty;
    mount(`project=${PROJECT_IDS.ledger}`);
    expect((await screen.findAllByText('0')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('–').length).toBeGreaterThan(0);
    expect(screen.getByText('Belum ada pencarian pada rentang ini')).toBeInTheDocument();
  });

  it('500 shows one error with trace id and retry; charts say unavailable', async () => {
    summaryStatus = 500;
    mount();
    expect(await screen.findByText('Ringkasan tidak dapat dimuat.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Coba lagi' })).toBeInTheDocument();
    expect(screen.getAllByText('Data tidak tersedia').length).toBeGreaterThanOrEqual(4);
  });

  it('project list failure does not block the summary', async () => {
    fetchMock.mockImplementation(async (url: string) =>
      url.startsWith('/api/projects')
        ? new Response(JSON.stringify({ error: { code: 'INTERNAL', message: 'x' }, trace_id: 't' }), { status: 500 })
        : new Response(JSON.stringify(mockSummary7d), { status: 200 }),
    );
    mount();
    expect(await screen.findByText('1.234')).toBeInTheDocument();
    expect(await screen.findByText('Daftar project gagal dimuat')).toBeInTheDocument();
  });

  const summaryCalls = () => fetchMock.mock.calls.filter((c) => String(c[0]).startsWith('/api/audit-log/summary'));

  it('empty query triggers exactly one summary fetch with range=7d', async () => {
    mount();
    await screen.findByText('1.234');
    expect(summaryCalls()).toHaveLength(1);
    expect(summaryCalls()[0]![0]).toBe('/api/audit-log/summary?range=7d');
  });

  it('a new query from the parent triggers one more fetch', async () => {
    const { rerender } = render(SummaryView, { query: new URLSearchParams(''), onquerychange: vi.fn() });
    await screen.findByText('1.234');
    expect(summaryCalls()).toHaveLength(1);
    await rerender({ query: new URLSearchParams('range=24h') });
    await waitFor(() => expect(summaryCalls()).toHaveLength(2));
    expect(summaryCalls()[1]![0]).toBe('/api/audit-log/summary?range=24h');
  });

  it('does not render an h1 (the page owns it)', async () => {
    mount();
    await screen.findByText('1.234');
    expect(screen.queryByRole('heading', { level: 1 })).toBeNull();
  });

  it('with a project the top-projects card is not rendered', async () => {
    mount();
    await screen.findByText('1.234');
    expect(screen.getByRole('table', { name: 'Project teratas' })).toBeInTheDocument();
    summary = mockSummary24hProject;
    cleanup();
    mount(`range=24h&project=${PROJECT_IDS.payments}`);
    await screen.findByText('4/59');
    expect(screen.queryByRole('table', { name: 'Project teratas' })).toBeNull();
    expect(screen.queryByText('Project teratas')).toBeNull();
  });

  it('400 offers "Reset filter" (no retry) which goes back to range=7d', async () => {
    summaryStatus = 400;
    const { onquerychange } = mount('range=24h');
    const reset = await screen.findByRole('button', { name: 'Reset filter' });
    expect(screen.queryByRole('button', { name: 'Coba lagi' })).toBeNull();
    await userEvent.click(reset);
    expect(onquerychange).toHaveBeenCalledTimes(1);
    expect(onquerychange.mock.calls[0]![0].toString()).toBe('range=7d');
  });
});
