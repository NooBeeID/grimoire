import { describe, expect, it } from 'vitest';
import { excerpt, formatCount, formatRelative } from './format';

describe('format', () => {
  it('formatCount', () => {
    expect(formatCount(null)).toBe('-');
    expect(formatCount(0)).toBe('0');
    expect(formatCount(1234, 'en')).toBe('1,234');
  });
  it('formatRelative falls back to raw string on invalid ISO', () => {
    expect(formatRelative('nope', 'id')).toBe('nope');
  });
  it('formatRelative gives relative text', () => {
    const now = new Date('2026-10-22T12:00:00Z');
    expect(formatRelative('2026-10-22T10:00:00Z', 'en', now)).toBe('2 hours ago');
  });
  it('excerpt cuts on rune boundary', () => {
    expect(excerpt('abc', 5)).toBe('abc');
    expect(excerpt('\u{1F44D}\u{1F44D}\u{1F44D}\u{1F44D}', 2)).toBe('\u{1F44D}\u{1F44D}…');
  });
});
