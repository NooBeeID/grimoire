import { describe, expect, it } from 'vitest';
import { formatBytes, utf8Length } from './bytes';

describe('bytes', () => {
  it('utf8Length counts bytes not code units', () => {
    expect(utf8Length('abc')).toBe(3);
    expect(utf8Length('é')).toBe(2);
  });
  it('formatBytes uses base 1024', () => {
    expect(formatBytes(512, 'en')).toBe('512 B');
    expect(formatBytes(2048, 'en')).toBe('2 KB');
    expect(formatBytes(3 * 1024 * 1024, 'en')).toBe('3 MB');
  });
});
