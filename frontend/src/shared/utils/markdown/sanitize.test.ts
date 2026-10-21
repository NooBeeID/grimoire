import { describe, expect, it } from 'vitest';
import { sanitizeHtml, sanitizeSvg } from './sanitize';

const dom = (html: string) => {
  const d = document.createElement('div');
  d.innerHTML = html;
  return d;
};

describe('sanitizeHtml', () => {
  it('T05-AC5: drops script, iframe, form, style and img elements', () => {
    const out = dom(
      sanitizeHtml(
        '<p>ok</p><script>alert(1)</script><iframe src="x"></iframe><form><input></form><style>p{}</style><img src=x onerror=alert(1)>',
      ),
    );
    expect(out.querySelector('script, iframe, form, style, img')).toBeNull();
    expect(out.textContent).toContain('ok');
  });

  it('T05-AC5: removes inline handlers and style attributes', () => {
    const out = dom(sanitizeHtml('<a href="https://x.test" onclick="alert(1)" style="color:red">x</a><p onmouseover="a()">y</p>'));
    const a = out.querySelector('a')!;
    expect(a.getAttribute('href')).toBe('https://x.test');
    expect(a.hasAttribute('onclick')).toBe(false);
    expect(a.hasAttribute('style')).toBe(false);
    expect(out.querySelector('p')!.hasAttribute('onmouseover')).toBe(false);
  });

  it.each(['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'data:text/html,<b>x</b>', 'vbscript:msgbox(1)'])(
    'T05-AC5: strips href %s',
    (href) => {
      const a = dom(sanitizeHtml(`<a href="${href}">x</a>`)).querySelector('a');
      expect(a?.getAttribute('href') ?? null).toBeNull();
    },
  );

  it.each(['https://x.test/a', 'mailto:a@b.id', '#bagian', '/projects/p1', './b.md', '../c.md', 'relatif.md'])(
    'keeps safe href %s',
    (href) => {
      expect(dom(sanitizeHtml(`<a href="${href}">x</a>`)).querySelector('a')!.getAttribute('href')).toBe(href);
    },
  );

  it('keeps allowed markup: headings, tables, code, mermaid marker attribute', () => {
    const out = dom(
      sanitizeHtml(
        '<h2 id="md-a">T</h2><table><thead><tr><th align="left">h</th></tr></thead><tbody><tr><td>c</td></tr></tbody></table><pre><code class="language-ts">x</code></pre><div class="md-mermaid" data-mermaid-index="0"></div>',
      ),
    );
    expect(out.querySelector('h2#md-a')).not.toBeNull();
    expect(out.querySelector('table th')!.getAttribute('align')).toBe('left');
    expect(out.querySelector('code.language-ts')).not.toBeNull();
    expect(out.querySelector('[data-mermaid-index="0"]')).not.toBeNull();
  });

  it('keeps task list checkboxes but forces them to stay disabled', () => {
    const cb = dom(sanitizeHtml('<ul><li><input type="checkbox" checked disabled> a</li></ul>')).querySelector('input')!;
    expect(cb.getAttribute('type')).toBe('checkbox');
    expect(cb.hasAttribute('checked')).toBe(true);
    expect(cb.hasAttribute('disabled')).toBe(true);
  });

  it('drops svg and math payloads embedded in markdown html', () => {
    const out = dom(sanitizeHtml('<svg onload="alert(1)"><circle r="1"/></svg><math><mi>x</mi></math>'));
    expect(out.querySelector('svg, math')).toBeNull();
  });
});

describe('sanitizeSvg', () => {
  it('T05-AC5: removes script, foreignObject and on* attributes from mermaid output', () => {
    const out = dom(
      sanitizeSvg(
        '<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(1)</script><foreignObject><div>x</div></foreignObject><g onclick="a()"><rect width="4" height="4"/></g></svg>',
      ),
    );
    expect(out.querySelector('script, foreignObject, foreignobject')).toBeNull();
    expect(out.querySelector('svg')!.hasAttribute('onload')).toBe(false);
    expect(out.querySelector('g')!.hasAttribute('onclick')).toBe(false);
    expect(out.querySelector('rect')).not.toBeNull();
  });

  it('T05-AC5: drops javascript: links inside svg', () => {
    const out = dom(
      sanitizeSvg('<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><a href="javascript:alert(1)"><text>x</text></a></svg>'),
    );
    const a = out.querySelector('a');
    expect(a?.getAttribute('href') ?? '').not.toContain('javascript');
  });

  it('keeps ordinary shapes and text', () => {
    const out = dom(sanitizeSvg('<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0L4 4"/><text>A</text></svg>'));
    expect(out.querySelector('path')!.getAttribute('d')).toBe('M0 0L4 4');
    expect(out.querySelector('text')!.textContent).toBe('A');
  });
});
