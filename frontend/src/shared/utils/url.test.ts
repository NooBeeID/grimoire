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

describe('absoluteUrl', () => {
  it('menggabungkan path relatif, mempertahankan https, menolak skema berbahaya', async () => {
    const { absoluteUrl } = await import('./url');
    expect(absoluteUrl('/login', 'https://g.tim.id')).toBe('https://g.tim.id/login');
    expect(absoluteUrl('https://grimoire.tim.id/login', 'https://g.tim.id')).toBe('https://grimoire.tim.id/login');
    expect(absoluteUrl('javascript:alert(1)', 'https://g.tim.id')).toBe('https://g.tim.id/login');
    expect(absoluteUrl('//evil.example', 'https://g.tim.id')).toBe('https://g.tim.id/login');
  });
});
