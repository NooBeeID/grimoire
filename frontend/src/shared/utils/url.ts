// href aman untuk URL dari konten tidak tepercaya: hanya http(s) absolut; selain itu null (tampilkan sebagai teks).
export function safeHref(raw: unknown): string | null {
  if (typeof raw !== 'string') return null;
  try {
    const u = new URL(raw);
    return u.protocol === 'http:' || u.protocol === 'https:' ? raw : null;
  } catch {
    return null;
  }
}

// URL absolut http(s) dikembalikan apa adanya; path relatif digabung dengan origin;
// skema lain (mis. javascript:) diganti origin + '/login'.
export function absoluteUrl(url: string, origin: string): string {
  const safe = safeHref(url);
  if (safe) return safe;
  if (url.startsWith('/') && !url.startsWith('//')) return origin + url;
  return origin + '/login';
}
