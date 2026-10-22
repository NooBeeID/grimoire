// Dipakai vi.mock('chart.js', ...) di tes: jsdom tidak punya canvas 2D.
export const created: { config: unknown; destroyed: boolean }[] = [];

class FakeChart {
  static defaults = { font: {} as Record<string, unknown>, animation: true, responsive: false, maintainAspectRatio: true };
  static register = () => {};
  readonly record: { config: unknown; destroyed: boolean };
  constructor(_canvas: unknown, config: unknown) {
    this.record = { config, destroyed: false };
    created.push(this.record);
  }
  destroy() {
    this.record.destroyed = true;
  }
}

export const chartJsMock = () => ({
  Chart: FakeChart,
  BarController: {},
  LineController: {},
  BarElement: {},
  LineElement: {},
  PointElement: {},
  CategoryScale: {},
  LinearScale: {},
  Tooltip: {},
  Legend: {},
});
