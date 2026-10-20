import type { Locale } from '../stores/i18n';

export function formatDateTime(iso: string, locale: Locale): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(locale === 'id' ? 'id-ID' : 'en-GB', { dateStyle: 'medium', timeStyle: 'short' }).format(d);
}
