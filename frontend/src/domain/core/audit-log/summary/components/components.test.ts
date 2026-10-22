import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { setLocale } from '../../../../../shared/stores/i18n';
import { theme } from '../../../../../shared/stores/theme';
import { created } from '../../../../../test-stubs/chartjs';
import { mockProjectOptions, mockSummary24hProject, mockSummary7d, mockSummaryEmpty, PROJECT_IDS } from '../mocks/summary';
import type { ToolMetric } from '../types';
import ChartCard from './ChartCard.svelte';
import ChartLegend from './ChartLegend.svelte';
import ProjectPicker from './ProjectPicker.svelte';
import RangeSegmented from './RangeSegmented.svelte';
import SearchQualityBlock from './SearchQualityBlock.svelte';
import SummaryFigures from './SummaryFigures.svelte';
import ToolLatencyChart from './ToolLatencyChart.svelte';
import TrafficChart from './TrafficChart.svelte';

vi.mock('chart.js', async () => (await import('../../../../../test-stubs/chartjs')).chartJsMock());

beforeEach(() => {
  setLocale('id');
  theme.set('light');
  created.length = 0;
});
afterEach(() => vi.unstubAllGlobals());

const noop = () => {};
const body = createRawSnippet(() => ({ render: () => '<canvas role="img" aria-label="grafik"></canvas>' }));
const table = { columns: ['Tool', 'Panggilan'], rows: [['read_file', '600'], ['list_files', '150']] };
const cardProps = (status: 'loading' | 'empty' | 'error' | 'ready') => ({
  title: 'Tool paling sering dipakai',
  status,
  emptyText: 'Belum ada panggilan MCP pada rentang ini',
  table,
  children: body,
});

describe('ChartCard', () => {
  it('loading: skeleton aria-hidden, no canvas, aria-busy', () => {
    const { container } = render(ChartCard, cardProps('loading'));
    expect(container.querySelector('canvas')).toBeNull();
    expect(container.querySelector('[aria-hidden="true"].bg-raised')).not.toBeNull();
    expect(container.querySelector('figure')).toHaveAttribute('aria-busy', 'true');
  });
  it('empty: empty text, title stays, no canvas', () => {
    const { container } = render(ChartCard, cardProps('empty'));
    expect(screen.getByText('Belum ada panggilan MCP pada rentang ini')).toBeInTheDocument();
    expect(screen.getByText('Tool paling sering dipakai')).toBeInTheDocument();
    expect(container.querySelector('canvas')).toBeNull();
  });
  it('error: says data unavailable', () => {
    const { container } = render(ChartCard, cardProps('error'));
    expect(screen.getByText('Data tidak tersedia')).toBeInTheDocument();
    expect(container.querySelector('canvas')).toBeNull();
  });
  it('ready: labelled canvas and an sr-only table with the same rows', () => {
    const { container } = render(ChartCard, cardProps('ready'));
    expect(screen.getByRole('img', { name: 'grafik' })).toBeInTheDocument();
    const rows = [...container.querySelectorAll('table.sr-only tbody tr')].map((tr) => [...tr.querySelectorAll('td')].map((td) => td.textContent));
    expect(rows).toEqual(table.rows);
  });
  it('plot is 180px on phones whatever height was asked for', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: noop, removeEventListener: noop }));
    const { container } = render(ChartCard, { ...cardProps('ready'), height: 220 });
    expect(container.querySelector<HTMLElement>('div.relative')!.style.height).toBe('180px');
  });
  it('plot keeps the requested height on wider screens', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: false, addEventListener: noop, removeEventListener: noop }));
    const { container } = render(ChartCard, { ...cardProps('ready'), height: 220 });
    expect(container.querySelector<HTMLElement>('div.relative')!.style.height).toBe('220px');
  });
});

describe('ChartLegend', () => {
  it('hatch swatch draws a <pattern>; line and bar do not', () => {
    const { container } = render(ChartLegend, {
      items: [
        { label: 'Berhasil', kind: 'bar', color: 'red' },
        { label: 'Error', kind: 'hatch', color: 'blue' },
      ],
    });
    expect(container.querySelectorAll('pattern')).toHaveLength(1);
    expect(screen.getByText('Error').querySelector('pattern')).not.toBeNull();
    expect(screen.getByText('Berhasil').querySelector('pattern')).toBeNull();
  });
});

describe('SummaryFigures', () => {
  const props = (over = {}) => ({ status: 'ready' as const, data: mockSummary7d, error: null, refreshing: false, onretry: noop, onreset: noop, ...over });
  it('shows count, rate and error count; agent chip sits in the first cell', () => {
    const { container } = render(SummaryFigures, props());
    expect(screen.getByText('1.234')).toBeInTheDocument();
    expect(screen.getByText('2,5%')).toBeInTheDocument();
    expect(screen.getByText('31 error')).toBeInTheDocument();
    const first = container.querySelector('dl > div')!;
    expect(first.querySelector('dt svg, dt [class*="accent"]')).not.toBeNull();
  });
  it('null error rate becomes a dash', () => {
    render(SummaryFigures, props({ data: mockSummaryEmpty }));
    expect(screen.getByText('–')).toBeInTheDocument();
    expect(screen.getByText('0')).toBeInTheDocument();
  });
  it('error state: one alert with a retry button calling back', async () => {
    const onretry = vi.fn();
    const error = new ApiError({ status: 500, code: 'INTERNAL', message: 'gagal', traceId: 'tr-1' });
    render(SummaryFigures, props({ status: 'error', data: null, error, onretry }));
    expect(screen.getAllByRole('alert')).toHaveLength(1);
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalledTimes(1);
  });
});

describe('SearchQualityBlock', () => {
  const props = (over = {}) => ({ quality: null, loading: false, retrying: false, onretry: noop, onpickproject: noop, ...over });
  it('null: invites to pick a project and calls back', async () => {
    const onpickproject = vi.fn();
    render(SearchQualityBlock, props({ onpickproject }));
    expect(screen.getByText('Pilih project untuk melihat kualitas pencarian')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Pilih project' }));
    expect(onpickproject).toHaveBeenCalledTimes(1);
  });
  it('unavailable: sentence and retry; button is loading while retrying', async () => {
    const onretry = vi.fn();
    const quality = { available: false as const, error_code: 'UNAVAILABLE' };
    const a = render(SearchQualityBlock, props({ quality, onretry }));
    expect(screen.getByText('Kualitas pencarian tidak dapat dimuat.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalledTimes(1);
    a.unmount();
    render(SearchQualityBlock, props({ quality, retrying: true }));
    expect(screen.getByRole('button', { name: /Coba lagi/ })).toBeDisabled();
  });
  it('available: shows rates and ratios', () => {
    render(SearchQualityBlock, props({ quality: mockSummary24hProject.search_quality }));
    for (const s of ['6,8%', '4/59', '56,4%', '31/55', '11,9%', '7/59']) expect(screen.getByText(s)).toBeInTheDocument();
  });
  it('denominator 0 shows a dash', () => {
    render(SearchQualityBlock, props({ quality: mockSummaryEmpty.search_quality }));
    expect(screen.getAllByText('–')).toHaveLength(3);
  });
  it('legacy searches show a note', () => {
    const q = { ...(mockSummary24hProject.search_quality as object), legacy_searches: 3 };
    render(SearchQualityBlock, props({ quality: q }));
    expect(screen.getByText('3 pencarian lama tanpa data peringkat tidak dihitung.')).toBeInTheDocument();
  });
});

describe('RangeSegmented', () => {
  it('three radios, 7 hari checked; ArrowRight selects 30 hari and reports it', async () => {
    const onchange = vi.fn();
    render(RangeSegmented, { value: '7d', onchange });
    const radios = screen.getAllByRole('radio');
    expect(radios).toHaveLength(3);
    expect(screen.getByLabelText('7 hari')).toBeChecked();
    screen.getByLabelText('7 hari').focus();
    await userEvent.keyboard('{ArrowRight}');
    expect(onchange).toHaveBeenCalledWith('30d');
    expect(screen.getByLabelText('30 hari')).toBeChecked();
  });
});

describe('ProjectPicker', () => {
  const props = (over = {}) => ({
    projectId: null as string | null,
    options: mockProjectOptions.items,
    total: 5,
    status: 'ready' as const,
    fallbackLabel: null as string | null,
    onchange: noop,
    onretry: noop,
    ...over,
  });
  it('"Semua project" plus five options', () => {
    render(ProjectPicker, props());
    expect(screen.getAllByRole('option')).toHaveLength(6);
    expect(screen.getByRole('option', { name: 'Semua project' })).toBeInTheDocument();
  });
  it('unknown projectId is inserted with fallbackLabel', () => {
    const id = '0192f5a0-9999-7aaa-8bbb-000000000009';
    render(ProjectPicker, props({ projectId: id, fallbackLabel: 'Billing Gateway' }));
    expect(screen.getAllByRole('option')).toHaveLength(7);
    expect(screen.getByRole('option', { name: 'Billing Gateway' })).toBeInTheDocument();
  });
  it('total above the listed options shows a hint', () => {
    render(ProjectPicker, props({ total: 140 }));
    expect(screen.getByText(/Menampilkan 5 project/)).toBeInTheDocument();
  });
  it('error: select disabled, retry button calls back', async () => {
    const onretry = vi.fn();
    render(ProjectPicker, props({ status: 'error', options: [], total: 0, onretry }));
    expect(screen.getByRole('combobox')).toBeDisabled();
    expect(screen.getByText('Daftar project gagal dimuat')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Coba lagi' }));
    expect(onretry).toHaveBeenCalledTimes(1);
  });
  it('selecting a project reports its id', async () => {
    const onchange = vi.fn();
    render(ProjectPicker, props({ onchange }));
    await userEvent.selectOptions(screen.getByRole('combobox'), PROJECT_IDS.payments);
    expect(onchange).toHaveBeenCalledWith(PROJECT_IDS.payments);
  });
});

describe('TrafficChart', () => {
  const props = (over = {}) => ({ points: mockSummary7d.traffic, bucket: 'day' as const, status: 'ready' as const, totalCalls: 1234, ...over });
  it('empty summary still draws the zero line and says so', () => {
    render(TrafficChart, props({ points: mockSummaryEmpty.traffic, status: 'ready', totalCalls: 0 }));
    expect(created).toHaveLength(1);
    expect((created[0]!.config as { data: { datasets: { data: number[] }[] } }).data.datasets[0]!.data.every((n) => n === 0)).toBe(true);
    expect(screen.getByText(/Belum ada panggilan MCP pada rentang ini/)).toBeInTheDocument();
  });
  it('theme change rebuilds the chart with re-read colours; unmount destroys it', async () => {
    const { unmount } = render(TrafficChart, props());
    expect(created).toHaveLength(1);
    document.documentElement.style.setProperty('--color-chart-1', 'rgb(1, 2, 3)');
    theme.set('dark');
    await waitFor(() => expect(created).toHaveLength(2));
    expect(created[0]!.destroyed).toBe(true);
    const ds = (created[1]!.config as { data: { datasets: { borderColor: string }[] } }).data.datasets;
    expect(ds[0]!.borderColor).toBe('rgb(1, 2, 3)');
    unmount();
    expect(created[1]!.destroyed).toBe(true);
    document.documentElement.style.removeProperty('--color-chart-1');
  });
  it('phones use at most 4 x ticks, wider screens 8', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true, addEventListener: noop, removeEventListener: noop }));
    const a = render(TrafficChart, props());
    const ticks = (i: number) => (created[i]!.config as { options: { scales: { x: { ticks: { maxTicksLimit: number } } } } }).options.scales.x.ticks.maxTicksLimit;
    expect(ticks(0)).toBe(4);
    a.unmount();
    vi.stubGlobal('matchMedia', () => ({ matches: false, addEventListener: noop, removeEventListener: noop }));
    render(TrafficChart, props());
    expect(ticks(created.length - 1)).toBe(8);
  });
});

describe('ToolLatencyChart', () => {
  it('all-null latency shows the dedicated empty text', () => {
    const rows: ToolMetric[] = mockSummary7d.tool_metrics.map((m) => ({ ...m, p50_ms: null, p95_ms: null }));
    render(ToolLatencyChart, { rows, status: 'ready' });
    expect(screen.getByText('Belum ada data latensi pada rentang ini')).toBeInTheDocument();
    expect(created).toHaveLength(0);
  });
});
