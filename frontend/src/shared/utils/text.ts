// Menghitung code point (setara utf8.RuneCountInString di backend).
export function runeLength(s: string): number {
  return [...s].length;
}

// true bila s memuat karakter kontrol (\p{Cc}) selain yang diizinkan.
export function hasControlChars(s: string, allow: string[] = []): boolean {
  for (const ch of s) {
    if (/\p{Cc}/u.test(ch) && !allow.includes(ch)) return true;
  }
  return false;
}

// Memotong ke maksimal max code point.
export function truncateRunes(s: string, max: number): string {
  const r = [...s];
  return r.length <= max ? s : r.slice(0, max).join('');
}
