export const MIN_TERMS = 2;
export const MAX_TERMS = 10;
export const MAX_TERM_CHARS = 64;

export type TermsIssue = 'tooFew' | 'tooMany' | 'empty' | 'tooLong' | 'invalidChar' | 'duplicate';

// Huruf, angka, garis bawah, dan spasi tunggal (cermin NormalizeTerms backend).
const ALLOWED = /^[\p{L}\p{N}_ ]+$/u;

// Huruf kecil, spasi diringkas dan dipangkas. Isu pertama yang ditemukan dikembalikan;
// `terms` selalu hasil normalisasi (termasuk saat ada isu) supaya UI bisa menampilkannya.
export function normalizeTerms(raw: string[]): { terms: string[]; issue: TermsIssue | null } {
  const terms = raw.map((r) => r.toLowerCase().replace(/\s+/g, ' ').trim());
  let issue: TermsIssue | null = null;
  if (terms.length < MIN_TERMS) issue = 'tooFew';
  else if (terms.length > MAX_TERMS) issue = 'tooMany';
  else if (terms.some((x) => x === '')) issue = 'empty';
  else if (terms.some((x) => [...x].length > MAX_TERM_CHARS)) issue = 'tooLong';
  else if (terms.some((x) => !ALLOWED.test(x))) issue = 'invalidChar';
  else if (new Set(terms).size !== terms.length) issue = 'duplicate';
  return { terms, issue };
}
