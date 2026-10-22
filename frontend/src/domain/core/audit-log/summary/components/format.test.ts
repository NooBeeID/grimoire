import { afterEach, describe, expect, it } from 'vitest';
import { bucketLabel, formatCount, formatMs, formatPercent, truncate } from './format';

describe('format', () => {
  it('counts and percent by locale', () => {
    expect(formatCount(1234, 'id')).toBe('1.234');
    expect(formatCount(1234, 'en')).toBe('1,234');
    expect(formatPercent(0.0251, 'id')).toBe('2,5%');
    expect(formatPercent(0, 'id')).toBe('0%');
    expect(formatPercent(null, 'id')).toBe('–');
    expect(formatPercent(0, 'id', 0)).toBe('–');
  });
  it('ms', () => {
    expect(formatMs(null, 'id')).toBe('–');
    expect(formatMs(40, 'id')).toBe('40 ms');
  });
  it('day bucket uses UTC date', () => {
    expect(bucketLabel('2026-10-16T00:00:00Z', 'day', 'id')).toBe('16 Okt');
    expect(bucketLabel('2026-10-16T00:00:00Z', 'day', 'en')).toBe('16 Oct');
  });
  it('hour bucket uses local time', () => {
    const d = new Date('2026-10-16T10:00:00Z');
    const expected = `${String(d.getHours()).padStart(2, '0')}.${String(d.getMinutes()).padStart(2, '0')}`;
    expect(bucketLabel('2026-10-16T10:00:00Z', 'hour', 'id')).toBe(expected);
  });
  it('truncate', () => {
    expect(truncate('a'.repeat(30))).toBe('a'.repeat(24) + '…');
    expect(truncate('short')).toBe('short');
  });
});
afterEach(() => undefined);
