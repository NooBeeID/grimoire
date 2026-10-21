import DOMPurify from 'dompurify';

const ALLOWED_TAGS = [
  'p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'blockquote', 'pre', 'code', 'em', 'strong', 'del', 'a',
  'table', 'thead', 'tbody', 'tr', 'th', 'td', 'hr', 'br', 'span', 'div', 'input',
];
const ALLOWED_ATTR = ['href', 'title', 'target', 'rel', 'id', 'class', 'data-mermaid-index', 'align', 'type', 'checked', 'disabled'];

// Satu-satunya sumber HTML untuk {@html} (rule 12.6).
export function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
    FORBID_TAGS: ['style', 'script', 'iframe', 'object', 'embed', 'form', 'img', 'svg', 'math'],
    FORBID_ATTR: ['style'],
    ALLOWED_URI_REGEXP: /^(?:https?:|mailto:|#|\/(?!\/)|\.{0,2}\/|[^:]*$)/i,
  });
}

// SVG hasil Mermaid (securityLevel strict). Tanpa foreignObject/script; atribut on* dan
// href berskema javascript: dibuang DOMPurify.
export function sanitizeSvg(svg: string): string {
  return DOMPurify.sanitize(svg, {
    USE_PROFILES: { svg: true, svgFilters: true },
    FORBID_TAGS: ['foreignObject', 'script'],
    FORBID_ATTR: ['style'],
  });
}
