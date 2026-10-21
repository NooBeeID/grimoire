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
