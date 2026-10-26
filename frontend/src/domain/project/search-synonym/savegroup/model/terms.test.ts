import { describe, expect, it } from 'vitest';
import { MAX_TERM_CHARS, normalizeTerms } from './terms';

describe('normalizeTerms', () => {
  it('lowercases and collapses whitespace', () => {
    expect(normalizeTerms(['  Login ', 'Single   Sign On'])).toEqual({ terms: ['login', 'single sign on'], issue: null });
  });
  it('flags a single term as tooFew', () => {
    expect(normalizeTerms(['login']).issue).toBe('tooFew');
    expect(normalizeTerms([]).issue).toBe('tooFew');
  });
  it('flags more than 10 terms', () => {
    expect(normalizeTerms(Array.from({ length: 11 }, (_, i) => `t${i}`)).issue).toBe('tooMany');
  });
  it('flags empty term after trimming', () => {
    expect(normalizeTerms(['login', '   ']).issue).toBe('empty');
  });
  it('flags overlong term by rune count', () => {
    expect(normalizeTerms(['login', 'a'.repeat(MAX_TERM_CHARS + 1)]).issue).toBe('tooLong');
    expect(normalizeTerms(['login', 'a'.repeat(MAX_TERM_CHARS)]).issue).toBeNull();
  });
  it('flags operator characters', () => {
    expect(normalizeTerms(['login', 'a:*']).issue).toBe('invalidChar');
    expect(normalizeTerms(['login', 'a|b']).issue).toBe('invalidChar');
  });
  it('flags duplicates after normalization', () => {
    expect(normalizeTerms(['Login', 'login']).issue).toBe('duplicate');
  });
  it('accepts digits, underscore and non-Latin letters', () => {
    expect(normalizeTerms(['oauth2', 'sso_login', 'masuk']).issue).toBeNull();
  });
});
