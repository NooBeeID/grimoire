import { describe, expect, it } from 'vitest';
import { formatDateTime } from './datetime';

describe('formatDateTime', () => {
  it('formats valid ISO per locale', () => {
    expect(formatDateTime('2026-10-21T09:00:00Z', 'en')).toMatch(/2026/);
    expect(formatDateTime('2026-10-21T09:00:00Z', 'id')).toMatch(/2026/);
  });
  it('returns input for invalid ISO', () => {
    expect(formatDateTime('bukan-tanggal', 'id')).toBe('bukan-tanggal');
  });
});
