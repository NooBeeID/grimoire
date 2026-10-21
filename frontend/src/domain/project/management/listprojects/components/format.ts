import type { Locale } from '../../../../../shared/stores/i18n';

export function formatCount(n: number | null, locale: Locale = 'id'): string {
  if (n === null) return '-';
  return new Intl.NumberFormat(locale === 'id' ? 'id-ID' : 'en-GB').format(n);
}

const UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 3600],
  ['month', 30 * 24 * 3600],
  ['day', 24 * 3600],
  ['hour', 3600],
  ['minute', 60],
];

export function formatRelative(iso: string, locale: Locale, now: Date = new Date()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const tag = locale === 'id' ? 'id-ID' : 'en-GB';
  const diff = Math.round((d.getTime() - now.getTime()) / 1000);
  const abs = Math.abs(diff);
  if (abs > 30 * 24 * 3600 * 6) return new Intl.DateTimeFormat(tag, { dateStyle: 'medium' }).format(d);
  const rtf = new Intl.RelativeTimeFormat(tag, { numeric: 'auto' });
  for (const [unit, secs] of UNITS) {
    if (abs >= secs) return rtf.format(Math.trunc(diff / secs), unit);
  }
  return rtf.format(0, 'minute');
}

export function excerpt(s: string, max = 140): string {
  const r = [...s];
  return r.length <= max ? s : r.slice(0, max).join('').trimEnd() + '…';
}
