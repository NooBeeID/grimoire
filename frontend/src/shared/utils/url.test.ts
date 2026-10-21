import { describe, expect, it } from 'vitest';
import { safeHref } from './url';

describe('safeHref', () => {
  it('allows http and https only', () => {
    expect(safeHref('https://a.b/x')).toBe('https://a.b/x');
    expect(safeHref('http://a')).toBe('http://a');
  });
  it('rejects dangerous or relative values', () => {
    for (const v of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'data:text/html,x', '//evil', '/relative', 'ftp://x', 42, null]) {
      expect(safeHref(v)).toBeNull();
    }
  });
});
