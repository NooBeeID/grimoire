import { describe, expect, it } from 'vitest';
import { isSafeHref, resolveRelativePath } from './links';

describe('links', () => {
  it('resolveRelativePath', () => {
    expect(resolveRelativePath('docs/auth', '../a.md')).toBe('docs/a.md');
    expect(resolveRelativePath('', '../x.md')).toBeNull();
    expect(resolveRelativePath('docs', './b.md#h')).toBe('docs/b.md');
  });
  it('isSafeHref allows only safe schemes', () => {
    for (const ok of ['https://x.id', 'http://x', 'mailto:a@b.id', '#sec', 'a/b.md', './c.md']) expect(isSafeHref(ok)).toBe(true);
    for (const bad of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'data:text/html,x', 'vbscript:x', '//evil.id']) expect(isSafeHref(bad)).toBe(false);
  });
});
