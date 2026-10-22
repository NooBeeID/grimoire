import { beforeEach, describe, expect, it, vi } from 'vitest';

const register = vi.hoisted(() => vi.fn());
vi.mock('chart.js', () => ({
  Chart: { register, defaults: { font: {} } },
  BarController: { n: 'BarController' },
  LineController: { n: 'LineController' },
  BarElement: { n: 'BarElement' },
  LineElement: { n: 'LineElement' },
  PointElement: { n: 'PointElement' },
  CategoryScale: { n: 'CategoryScale' },
  LinearScale: { n: 'LinearScale' },
  Tooltip: { n: 'Tooltip' },
  Legend: { n: 'Legend' },
}));

beforeEach(() => {
  register.mockClear();
  vi.resetModules();
});

describe('setupCharts', () => {
  it('registers exactly nine components, once', async () => {
    const { setupCharts } = await import('./chartSetup');
    setupCharts();
    setupCharts();
    expect(register).toHaveBeenCalledTimes(1);
    expect(register.mock.calls[0]).toHaveLength(9);
    expect(new Set(register.mock.calls[0]!.map((c: { n: string }) => c.n)).size).toBe(9);
  });
  it('importing the module twice still registers once', async () => {
    const a = await import('./chartSetup');
    const b = await import('./chartSetup');
    a.setupCharts();
    b.setupCharts();
    expect(register).toHaveBeenCalledTimes(1);
    expect(register.mock.calls[0]).toHaveLength(9);
  });
});
