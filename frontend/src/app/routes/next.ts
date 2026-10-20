// Hanya path relatif same-origin; mencegah open redirect.
const BLOCKED = new Set(['/login', '/change-password']);

export function safeNext(raw: string | null): string | null {
  if (!raw || raw.length > 2048) return null;
  if (!raw.startsWith('/') || raw.startsWith('//') || raw.startsWith('/\\')) return null;
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f\\]/.test(raw)) return null;
  const path = raw.split(/[?#]/)[0]!;
  if (BLOCKED.has(path.replace(/\/+$/, '') || '/')) return null;
  return raw;
}
