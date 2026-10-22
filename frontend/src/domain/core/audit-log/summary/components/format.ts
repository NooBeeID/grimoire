import type { Locale } from '../../../../../shared/stores/i18n';

const MONTHS: Record<Locale, string[]> = {
  id: ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'],
  en: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
};
const DASH = '–';
const tag = (l: Locale) => (l === 'id' ? 'id-ID' : 'en-US');
const pad = (n: number) => String(n).padStart(2, '0');

export const formatCount = (n: number, locale: Locale) => new Intl.NumberFormat(tag(locale)).format(n);

export function formatPercent(rate: number | null, locale: Locale, denominator?: number): string {
  if (rate === null || denominator === 0) return DASH;
  return new Intl.NumberFormat(tag(locale), { style: 'percent', maximumFractionDigits: 1 }).format(rate);
}

export const formatMs = (v: number | null, locale: Locale) => (v === null ? DASH : `${formatCount(v, locale)} ms`);

export function bucketLabel(at: string, bucket: 'hour' | 'day', locale: Locale): string {
  const d = new Date(at);
  if (bucket === 'day') return `${d.getUTCDate()} ${MONTHS[locale][d.getUTCMonth()]}`;
  return `${pad(d.getHours())}${locale === 'id' ? '.' : ':'}${pad(d.getMinutes())}`;
}

export function formatClock(d: Date, locale: Locale): string {
  return `${pad(d.getHours())}${locale === 'id' ? '.' : ':'}${pad(d.getMinutes())}`;
}

export function utcOffsetCaption(d = new Date()): string {
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  const off = -d.getTimezoneOffset();
  const sign = off >= 0 ? '+' : '-';
  const abs = Math.abs(off);
  const hh = Math.floor(abs / 60);
  const mm = abs % 60;
  return `${zone} (UTC${sign}${hh}${mm ? ':' + pad(mm) : ''})`;
}

export function truncate(label: string, max = 24): string {
  return label.length > max ? label.slice(0, max) + '…' : label;
}
