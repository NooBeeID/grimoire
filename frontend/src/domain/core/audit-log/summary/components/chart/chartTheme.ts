export interface ChartTheme {
  chart1: string;
  chart2: string;
  danger: string;
  muted: string;
  rule: string;
  fg: string;
  surface: string;
  /** Isian segmen error: pola diagonal bila canvas tersedia, kalau tidak warna danger polos. */
  errorFill: string | CanvasPattern;
}

/** Pola garis diagonal 45 derajat, 6px. Dipakai dataset dan swatch legenda agar sama. */
export function hatch(color: string, bg: string): string | CanvasPattern {
  if (typeof document === 'undefined') return color;
  try {
    const c = document.createElement('canvas');
    c.width = c.height = 6;
    const ctx = c.getContext('2d');
    if (!ctx) return color;
    ctx.fillStyle = bg;
    ctx.fillRect(0, 0, 6, 6);
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(0, 6);
    ctx.lineTo(6, 0);
    ctx.stroke();
    return ctx.createPattern(c, 'repeat') ?? color;
  } catch {
    return color;
  }
}

export function readChartTheme(): ChartTheme {
  const cs = getComputedStyle(document.documentElement);
  const v = (name: string) => cs.getPropertyValue(name).trim() || 'currentColor';
  const danger = v('--color-danger');
  const surface = v('--color-surface');
  return {
    chart1: v('--color-chart-1'),
    chart2: v('--color-chart-2'),
    danger,
    muted: v('--color-muted'),
    rule: v('--color-rule'),
    fg: v('--color-fg'),
    surface,
    errorFill: hatch(danger, surface),
  };
}
