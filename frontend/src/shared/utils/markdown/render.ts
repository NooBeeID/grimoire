import { Marked, type Tokens } from 'marked';
import { isExternalHref, isSafeHref } from './links';
import { sanitizeHtml } from './sanitize';

const esc = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

export interface RenderOptions {
  // Tautan relatif -> href baru, atau null untuk teks saja.
  resolveLink?: (href: string) => string | null;
}
export interface Rendered {
  html: string;
  mermaid: string[];
}

export function renderMarkdown(src: string, o: RenderOptions = {}): Rendered {
  const mermaid: string[] = [];
  const md = new Marked({ gfm: true, breaks: false, async: false });
  md.use({
    renderer: {
      html: ({ text }: Tokens.HTML | Tokens.Tag) => esc(text),
      image: ({ text }: Tokens.Image) => `<span class="md-img-alt">[${esc(text)}]</span>`,
      code({ text, lang }: Tokens.Code) {
        if ((lang ?? '').trim() === 'mermaid') {
          const i = mermaid.push(text) - 1;
          return `<div class="md-mermaid" data-mermaid-index="${i}"><pre><code>${esc(text)}</code></pre></div>`;
        }
        return `<pre><code>${esc(text)}</code></pre>`;
      },
      heading({ tokens, depth }: Tokens.Heading) {
        const inner = this.parser.parseInline(tokens);
        const slug = inner.replace(/<[^>]*>/g, '').toLowerCase().trim().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-|-$/g, '');
        return `<h${depth} id="md-${esc(slug)}">${inner}</h${depth}>`;
      },
      link({ href, title, tokens }: Tokens.Link) {
        const inner = this.parser.parseInline(tokens);
        if (!isSafeHref(href)) return inner;
        let target = href;
        if (!isExternalHref(href) && !href.startsWith('#') && o.resolveLink) {
          const r = o.resolveLink(href);
          if (r === null) return inner;
          target = r;
        }
        const t = title ? ` title="${esc(title)}"` : '';
        const ext = isExternalHref(href) ? ' target="_blank" rel="noopener noreferrer nofollow"' : '';
        return `<a href="${esc(target)}"${t}${ext}>${inner}</a>`;
      },
    },
  });
  const raw = md.parse(src) as string;
  return { html: sanitizeHtml(raw), mermaid };
}
