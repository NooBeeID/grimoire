export function utf8Length(s: string): number {
  return new TextEncoder().encode(s).length;
}

// Basis 1024; satu angka desimal hanya bila perlu.
export function formatBytes(n: number, locale: 'id' | 'en' = 'id'): string {
  const fmt = new Intl.NumberFormat(locale === 'id' ? 'id-ID' : 'en-GB', { maximumFractionDigits: 1 });
  if (n < 1024) return `${fmt.format(n)} B`;
  if (n < 1024 * 1024) return `${fmt.format(n / 1024)} KB`;
  return `${fmt.format(n / (1024 * 1024))} MB`;
}
