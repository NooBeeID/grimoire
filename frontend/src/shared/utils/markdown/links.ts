const SAFE_SCHEMES = new Set(['http:', 'https:', 'mailto:']);

// true untuk http(s), mailto, fragmen (#x), atau tautan relatif tanpa skema.
export function isSafeHref(href: string): boolean {
  const h = href.trim();
  if (h === '') return false;
  if (h.startsWith('#')) return true;
  if (h.startsWith('//')) return false;
  const m = /^([a-zA-Z][a-zA-Z0-9+.-]*:)/.exec(h);
  if (!m) return true;
  return SAFE_SCHEMES.has(m[1]!.toLowerCase());
}

export function isExternalHref(href: string): boolean {
  return /^(https?:|mailto:)/i.test(href.trim());
}

// Gabungkan baseDir dengan href relatif; ".." dihitung; keluar dari root -> null.
export function resolveRelativePath(baseDir: string, href: string): string | null {
  const clean = href.split('#')[0]!.split('?')[0]!;
  if (clean === '') return null;
  const parts = clean.startsWith('/') ? [] : baseDir.split('/').filter(Boolean);
  for (const seg of clean.split('/')) {
    if (seg === '' || seg === '.') continue;
    if (seg === '..') {
      if (parts.length === 0) return null;
      parts.pop();
    } else parts.push(seg);
  }
  return parts.length ? parts.join('/') : null;
}
