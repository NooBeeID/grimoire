import type { ChartConfiguration } from 'chart.js';
import type { Locale } from '../../../../../../shared/stores/i18n';
import type { TopProject, ToolMetric, TrafficPoint } from '../../types';
import { bucketLabel, truncate } from '../format';
import type { ChartTheme } from './chartTheme';

const MONO = '"IBM Plex Mono", ui-monospace, monospace';

const tickBase = (theme: ChartTheme) => ({ color: theme.muted, font: { size: 12 } });

function valueScale(theme: ChartTheme, horizontal: boolean, extra: Record<string, unknown> = {}) {
  return {
    beginAtZero: true,
    border: { display: false },
    grid: { color: theme.rule, lineWidth: 1, display: true },
    ticks: { ...tickBase(theme), precision: 0, maxRotation: 0, ...extra },
    ...(horizontal ? {} : {}),
  };
}

function categoryScale(theme: ChartTheme, opts: { mono?: boolean; grid?: boolean; maxTicks?: number } = {}) {
  return {
    border: { display: false },
    grid: { display: opts.grid ?? false },
    ticks: {
      ...tickBase(theme),
      ...(opts.mono ? { font: { size: 12, family: MONO } } : {}),
      maxRotation: 0,
      autoSkip: opts.maxTicks !== undefined,
      ...(opts.maxTicks !== undefined ? { maxTicksLimit: opts.maxTicks } : {}),
    },
  };
}

const common = { plugins: { legend: { display: false } } };

export function trafficConfig(
  points: TrafficPoint[],
  bucket: 'hour' | 'day',
  locale: Locale,
  theme: ChartTheme,
  maxTicks = 8,
): ChartConfiguration {
  return {
    type: 'line',
    data: {
      labels: points.map((p) => bucketLabel(p.at, bucket, locale)),
      datasets: [
        {
          label: 'calls',
          data: points.map((p) => p.calls),
          borderColor: theme.chart1,
          backgroundColor: theme.chart1,
          borderWidth: 2,
          tension: 0,
          pointRadius: 0,
          pointHoverRadius: 4,
        },
        {
          label: 'errors',
          data: points.map((p) => p.errors),
          borderColor: theme.danger,
          backgroundColor: theme.danger,
          borderWidth: 1.5,
          tension: 0,
          pointRadius: 0,
          pointHoverRadius: 4,
        },
      ],
    },
    options: {
      ...common,
      interaction: { mode: 'index', intersect: false },
      scales: { x: categoryScale(theme, { maxTicks }), y: valueScale(theme, false) },
    },
  } as ChartConfiguration;
}

export function topProjectsConfig(
  rows: TopProject[],
  theme: ChartTheme,
  onpick?: (projectId: string) => void,
): ChartConfiguration {
  return {
    type: 'bar',
    data: {
      labels: rows.map((r) => truncate(r.project_label)),
      datasets: [{ label: 'calls', data: rows.map((r) => r.calls), backgroundColor: theme.chart1, borderRadius: 0 }],
    },
    options: {
      indexAxis: 'y',
      plugins: {
        legend: { display: false },
        tooltip: { callbacks: { title: (items: { dataIndex: number }[]) => rows[items[0]?.dataIndex ?? 0]?.project_label ?? '' } },
      },
      onClick: (_e: unknown, elements: { index: number }[]) => {
        const el = elements[0];
        const row = el ? rows[el.index] : undefined;
        if (row && onpick) onpick(row.project_id);
      },
      scales: { x: valueScale(theme, true), y: categoryScale(theme) },
    },
  } as ChartConfiguration;
}

export function toolUsageConfig(rows: { tool: string; calls: number }[], theme: ChartTheme): ChartConfiguration {
  return {
    type: 'bar',
    data: {
      labels: rows.map((r) => truncate(r.tool)),
      datasets: [{ label: 'calls', data: rows.map((r) => r.calls), backgroundColor: theme.chart1, borderRadius: 0 }],
    },
    options: {
      indexAxis: 'y',
      ...common,
      scales: { x: valueScale(theme, true), y: categoryScale(theme, { mono: true }) },
    },
  } as ChartConfiguration;
}

export function toolStatusConfig(metrics: ToolMetric[], theme: ChartTheme): ChartConfiguration {
  return {
    type: 'bar',
    data: {
      labels: metrics.map((m) => truncate(m.tool)),
      datasets: [
        { label: 'ok', data: metrics.map((m) => m.ok), backgroundColor: theme.chart1, borderColor: theme.surface, borderWidth: 2, borderRadius: 0 },
        { label: 'error', data: metrics.map((m) => m.error), backgroundColor: theme.errorFill, borderColor: theme.surface, borderWidth: 2, borderRadius: 0 },
      ],
    },
    options: {
      indexAxis: 'y',
      ...common,
      scales: {
        x: { ...valueScale(theme, true), stacked: true },
        y: { ...categoryScale(theme, { mono: true }), stacked: true },
      },
    },
  } as ChartConfiguration;
}

export function toolLatencyConfig(metrics: ToolMetric[], theme: ChartTheme): ChartConfiguration {
  return {
    type: 'bar',
    data: {
      labels: metrics.map((m) => truncate(m.tool)),
      datasets: [
        { label: 'p50', data: metrics.map((m) => m.p50_ms), backgroundColor: theme.chart1, borderRadius: 0 },
        { label: 'p95', data: metrics.map((m) => m.p95_ms), backgroundColor: theme.chart2, borderRadius: 0 },
      ],
    },
    options: {
      indexAxis: 'y',
      ...common,
      scales: {
        x: { ...valueScale(theme, true), title: { display: true, text: 'ms', color: theme.muted, font: { size: 12 } } },
        y: categoryScale(theme, { mono: true }),
      },
    },
  } as ChartConfiguration;
}

export function rankConfig(
  buckets: { label: string; count: number }[],
  theme: ChartTheme,
  t: (key: string) => string,
): ChartConfiguration {
  const labels = buckets.map((b) => (b.label === 'not_in_results' ? t('dashboard.quality.notInResults') : b.label));
  return {
    type: 'bar',
    data: {
      labels,
      datasets: [
        {
          label: t('dashboard.quality.rank'),
          data: buckets.map((b) => b.count),
          backgroundColor: buckets.map((b) => (b.label === 'not_in_results' ? theme.muted : theme.chart1)),
          borderRadius: 0,
        },
      ],
    },
    options: { ...common, scales: { x: categoryScale(theme), y: valueScale(theme, false) } },
  } as ChartConfiguration;
}
