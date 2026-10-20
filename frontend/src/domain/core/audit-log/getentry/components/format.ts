export function formatJSON(v: unknown): string {
  try {
    const s = JSON.stringify(v, null, 2);
    return s === undefined ? String(v) : s;
  } catch {
    return String(v);
  }
}

export function formatDateTime(iso: string, locale: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'medium' }).format(d);
}
