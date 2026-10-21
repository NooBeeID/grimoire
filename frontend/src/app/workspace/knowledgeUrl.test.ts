import { describe, expect, it } from 'vitest';
import { buildKnowledgeHref, parseKnowledgeQuery } from './knowledgeUrl';

const parse = (s: string) => parseKnowledgeQuery(new URLSearchParams(s));

describe('knowledgeUrl', () => {
  it('round-trips path, spaces and unicode', () => {
    const href = buildKnowledgeHref('p1', { path: 'catatan rapat/é.md' });
    expect(href).toBe('/projects/p1/knowledge?path=catatan+rapat%2F%C3%A9.md');
    expect(parse(href.split('?')[1]!).path).toBe('catatan rapat/é.md');
  });
  it('mode=edit without path falls back to view; new=1 wins over path', () => {
    expect(parse('mode=edit').mode).toBe('view');
    expect(parse('mode=edit&path=a.md').mode).toBe('edit');
    const s = parse('new=1&path=a.md&dir=docs');
    expect(s).toMatchObject({ mode: 'new', path: null, dir: 'docs' });
  });
  it('search state', () => {
    expect(parse('q=login&in=docs')).toMatchObject({ q: 'login', scope: 'docs' });
    expect(buildKnowledgeHref('p1', { q: 'login', scope: 'docs' })).toBe('/projects/p1/knowledge?q=login&in=docs');
  });
  it('param order is stable and empty state has no query', () => {
    expect(buildKnowledgeHref('p1', {})).toBe('/projects/p1/knowledge');
    expect(buildKnowledgeHref('p1', { path: 'a.md', mode: 'edit', q: 'x' })).toBe('/projects/p1/knowledge?path=a.md&mode=edit&q=x');
    expect(buildKnowledgeHref('p/1', { mode: 'new', dir: 'docs' })).toBe('/projects/p%2F1/knowledge?new=1&dir=docs');
  });
});
