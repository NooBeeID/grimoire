import { describe, expect, it } from 'vitest';
import { SearchResultSchema } from './types';

describe('SearchResultSchema (T20 additive fields)', () => {
  it('accepts an old response without section/expanded', () => {
    const r = SearchResultSchema.parse({
      items: [{ path: 'docs/a.md', title: 'A', score: 1, snippet: [{ text: 'x', highlight: false }] }],
      total: 1,
    });
    expect(r.items[0]!.section).toBeUndefined();
    expect(r.expanded).toBeUndefined();
  });
  it('keeps section and expanded when present', () => {
    const r = SearchResultSchema.parse({
      items: [{ path: 'docs/a.md', title: 'A', score: 1, snippet: [], section: 'Refresh token' }],
      total: 1,
      expanded: true,
    });
    expect(r.items[0]!.section).toBe('Refresh token');
    expect(r.expanded).toBe(true);
  });
});
