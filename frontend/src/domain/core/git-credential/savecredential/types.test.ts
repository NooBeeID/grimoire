import { describe, expect, it } from 'vitest';
import { SavedCredentialSchema, scopeFor, validateToken } from './types';

describe('validateToken', () => {
  it('length bounds', () => {
    expect(validateToken('1234567')).toBe('length');
    expect(validateToken('a'.repeat(513))).toBe('length');
    expect(validateToken('a'.repeat(512))).toBeNull();
    expect(validateToken('12345678')).toBeNull();
  });
  it('charset', () => {
    expect(validateToken('abcd efgh')).toBe('charset');
    expect(validateToken('abcdefgé1')).toBe('charset');
  });
  it('trims surrounding whitespace', () => expect(validateToken('  ghp_abcdefgh  ')).toBeNull());
});
describe('scopeFor', () => {
  it('per provider', () => {
    expect(scopeFor('github')).toEqual(['repo']);
    expect(scopeFor('gitlab')).toEqual(['read_api', 'read_repository']);
  });
});
describe('SavedCredentialSchema', () => {
  it('rejects a response carrying the token', () => {
    const base = { provider: 'github', host: 'github.com', last4: 'a9F3', updated_at: '2026-10-22T07:10:00Z', replaced: false };
    expect(SavedCredentialSchema.safeParse(base).success).toBe(true);
    expect(SavedCredentialSchema.safeParse({ ...base, token: 'ghp_x' }).success).toBe(false);
  });
});
