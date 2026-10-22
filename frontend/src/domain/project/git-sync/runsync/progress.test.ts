import { describe, expect, it } from 'vitest';
import { announceBucket, percentOf, remainingSeconds, truncateMiddle } from './progress';

describe('progress helpers', () => {
  it('remainingSeconds rounds up and never goes negative', () => {
    expect(remainingSeconds(38_000, 0)).toBe(38);
    expect(remainingSeconds(38_000, 1000)).toBe(37);
    expect(remainingSeconds(37_200, 0)).toBe(38);
    expect(remainingSeconds(1000, 5000)).toBe(0);
  });
  it('percentOf', () => {
    expect(percentOf(7, 42)).toBe(16);
    expect(percentOf(0, 0)).toBe(100);
    expect(percentOf(43, 42)).toBe(100);
  });
  it('truncateMiddle keeps file name and start', () => {
    const r = truncateMiddle('docs/very/long/nested/folder/structure/auth/login-flow.md', 32);
    expect(r.endsWith('login-flow.md')).toBe(true);
    expect(r.startsWith('docs/')).toBe(true);
    expect(r).toContain('…');
    expect(r.length).toBeLessThanOrEqual(32);
    expect(truncateMiddle('docs/a.md')).toBe('docs/a.md');
    expect(truncateMiddle('docs/auth/login-flow.md', 16)).toBe('docs/…/login-flow.md');
  });
  it('announceBucket', () => {
    expect(announceBucket(38)).toBe(40);
    expect(announceBucket(35)).toBe(35);
  });
});
