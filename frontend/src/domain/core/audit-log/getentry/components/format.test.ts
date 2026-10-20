import { describe, expect, it } from 'vitest';
import { formatDateTime, formatJSON } from './format';

describe('format', () => {
  it('pretty prints with 2 spaces', () => {
    expect(formatJSON({ a: { b: 1 } })).toBe('{\n  "a": {\n    "b": 1\n  }\n}');
  });
  it('falls back without throwing', () => {
    const c: Record<string, unknown> = {};
    c.self = c;
    expect(() => formatJSON(c)).not.toThrow();
    expect(formatJSON(10n)).toBe('10');
  });
  it('formats dates per locale and passes through invalid', () => {
    expect(formatDateTime('2026-10-19T08:14:22Z', 'en')).not.toBe(formatDateTime('2026-10-19T08:14:22Z', 'id'));
    expect(formatDateTime('nope', 'en')).toBe('nope');
  });
});
