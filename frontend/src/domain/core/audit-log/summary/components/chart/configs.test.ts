import { describe, expect, it, vi } from 'vitest';
import { mockSummary24hProject, mockSummary7d } from '../../mocks/summary';
import type { ChartTheme } from './chartTheme';
import { rankConfig, toolLatencyConfig, toolStatusConfig, topProjectsConfig, trafficConfig } from './configs';

const theme: ChartTheme = {
  chart1: 'c1',
  chart2: 'c2',
  danger: 'dg',
  muted: 'mu',
  rule: 'ru',
  fg: 'fg',
  surface: 'su',
  errorFill: 'hatch',
};

describe('chart configs', () => {
  it('traffic: labels, series styles, scales', () => {
    const cfg = trafficConfig(mockSummary7d.traffic, 'day', 'id', theme);
    expect(cfg.data.labels).toHaveLength(8);
    const [calls, errors] = cfg.data.datasets as unknown as Record<string, any>[];
    expect(calls!).toMatchObject({ borderColor: 'c1', borderWidth: 2, tension: 0, pointRadius: 0 });
    expect(errors!).toMatchObject({ borderColor: 'dg', borderWidth: 1.5 });
    expect(errors!.borderDash).toBeUndefined();
    const scales = cfg.options!.scales as Record<string, any>;
    expect(scales.y).toMatchObject({ beginAtZero: true, grid: { color: 'ru' }, border: { display: false } });
    expect(scales.x.grid.display).toBe(false);
    expect(scales.x.ticks).toMatchObject({ maxRotation: 0, maxTicksLimit: 8 });
    expect((cfg.options as any).plugins.legend.display).toBe(false);
    expect(trafficConfig(mockSummary24hProject.traffic, 'hour', 'id', theme).data.labels).toHaveLength(25);
  });

  it('top projects: horizontal, truncated label, onpick', () => {
    const rows = [
      { project_id: 'a', project_label: 'x'.repeat(40), calls: 3 },
      { project_id: 'b', project_label: 'Short', calls: 2 },
    ];
    const onpick = vi.fn();
    const cfg = topProjectsConfig(rows, theme, onpick);
    expect((cfg.options as any).indexAxis).toBe('y');
    expect(cfg.data.labels![0]).toBe('x'.repeat(24) + '…');
    const title = (cfg.options as any).plugins.tooltip.callbacks.title;
    expect(title([{ dataIndex: 0 }])).toBe('x'.repeat(40));
    (cfg.options as any).onClick({}, [{ index: 1 }]);
    expect(onpick).toHaveBeenCalledWith('b');
  });

  it('status: stacked, error uses errorFill, sums equal calls', () => {
    const cfg = toolStatusConfig(mockSummary7d.tool_metrics, theme);
    const [ok, err] = cfg.data.datasets as unknown as { data: number[]; backgroundColor: unknown; borderColor: string; borderWidth: number }[] as [any, any];
    expect(ok.backgroundColor).toBe('c1');
    expect(err.backgroundColor).toBe('hatch');
    expect(err).toMatchObject({ borderColor: 'su', borderWidth: 2 });
    mockSummary7d.tool_metrics.forEach((m, i) => expect(ok.data[i] + err.data[i]).toBe(m.calls));
    expect((cfg.options as any).scales.x.stacked).toBe(true);
  });

  it('latency: null stays null, two series, ms axis', () => {
    const cfg = toolLatencyConfig(mockSummary7d.tool_metrics, theme);
    const [p50, p95] = cfg.data.datasets as unknown as { data: (number | null)[]; backgroundColor: string }[] as [any, any];
    expect(p50.data.at(-1)).toBeNull();
    expect([p50.backgroundColor, p95.backgroundColor]).toEqual(['c1', 'c2']);
    expect((cfg.options as any).scales.x.title.text).toBe('ms');
  });

  it('rank: 11 labels, last muted', () => {
    const q = mockSummary24hProject.search_quality as { rank_opened: { label: string; count: number }[] };
    const cfg = rankConfig(q.rank_opened, theme, (k) => (k.endsWith('notInResults') ? 'tidak di hasil' : k));
    expect(cfg.data.labels).toHaveLength(11);
    expect(cfg.data.labels!.at(-1)).toBe('tidak di hasil');
    const bg = (cfg.data.datasets[0] as any).backgroundColor as string[];
    expect(bg.at(-1)).toBe('mu');
    expect(bg[0]).toBe('c1');
  });
});
