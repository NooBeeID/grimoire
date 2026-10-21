import { describe, expect, it } from 'vitest';
import { renderMarkdown } from './render';

const html = (s: string, o = {}) => renderMarkdown(s, o).html;
const dom = (s: string, o = {}) => {
  const d = document.createElement('div');
  d.innerHTML = html(s, o);
  return d;
};

describe('renderMarkdown (T05-AC5)', () => {
  it('raw script becomes text, no script element', () => {
    const d = dom('<script>alert(1)</script>');
    expect(d.querySelector('script')).toBeNull();
    expect(d.textContent).toContain('alert(1)');
  });
  it('dangerous link schemes are dropped to text', () => {
    for (const h of ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'data:text/html,x', 'vbscript:x']) {
      const d = dom(`[klik](${h})`);
      expect(d.querySelector('a')).toBeNull();
      expect(d.textContent).toContain('klik');
    }
  });
  it('raw img with onerror yields no img nor handler', () => {
    const d = dom('<img src=x onerror=alert(1)>');
    expect(d.querySelector('img')).toBeNull();
    expect(html('<img src=x onerror=alert(1)>')).not.toMatch(/<[^>]+onerror/);
  });
  it('markdown image becomes alt text only', () => {
    const d = dom('![diagram](x.png)');
    expect(d.querySelector('img')).toBeNull();
    expect(d.textContent).toContain('[diagram]');
  });
  it('external links get rel and target; raw onclick is escaped', () => {
    const a = dom('[x](https://x.id)').querySelector('a')!;
    expect(a.getAttribute('rel')).toBe('noopener noreferrer nofollow');
    expect(a.getAttribute('target')).toBe('_blank');
    expect(dom('<a href="https://x" onclick="e()">z</a>').querySelector('[onclick]')).toBeNull();
  });
  it('heading ids are prefixed md-', () => {
    expect(dom('# Alur Login').querySelector('h1')!.id).toBe('md-alur-login');
  });
  it('gfm table and task list (checkbox disabled)', () => {
    const d = dom('| a | b |\n|---|---|\n| 1 | 2 |\n\n- [x] done');
    expect(d.querySelector('table')).not.toBeNull();
    expect(d.querySelector('input[type=checkbox]')?.hasAttribute('disabled')).toBe(true);
  });
  it('mermaid fence becomes placeholder and is recorded', () => {
    const r = renderMarkdown('```mermaid\ngraph TD; A-->B\n```');
    expect(r.mermaid).toEqual(['graph TD; A-->B']);
    expect(r.html).toContain('data-mermaid-index="0"');
  });
  it('relative links go through resolveLink; null -> text', () => {
    const ok = dom('[a](b.md)', { resolveLink: () => '/projects/p/knowledge?path=b.md' }).querySelector('a')!;
    expect(ok.getAttribute('href')).toBe('/projects/p/knowledge?path=b.md');
    expect(dom('[a](b.md)', { resolveLink: () => null }).querySelector('a')).toBeNull();
  });
});
