import { describe, expect, it } from 'vitest';
import { hasControlChars, runeLength, truncateRunes } from './text';

describe('text utils', () => {
  it('runeLength counts code points', () => {
    expect(runeLength('\u{1F44D}a')).toBe(2);
    expect(runeLength('')).toBe(0);
  });
  it('hasControlChars', () => {
    expect(hasControlChars('a\nb')).toBe(true);
    expect(hasControlChars('a\nb', ['\n'])).toBe(false);
    expect(hasControlChars('a\u0000')).toBe(true);
    expect(hasControlChars('abc \u{1F44D}')).toBe(false);
  });
  it('truncateRunes cuts on code point boundary', () => {
    expect(truncateRunes('\u{1F44D}\u{1F44D}\u{1F44D}', 2)).toBe('\u{1F44D}\u{1F44D}');
    expect(truncateRunes('abc', 5)).toBe('abc');
  });
});
