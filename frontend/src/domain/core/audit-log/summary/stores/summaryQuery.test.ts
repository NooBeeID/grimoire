import { describe, expect, it } from 'vitest';
import { PROJECT_IDS } from '../mocks/summary';
import { summaryFilterFromQuery, summaryFilterToQuery } from './summaryQuery';

const q = (s: string) => new URLSearchParams(s);

describe('summaryQuery', () => {
  it('defaults', () => {
    expect(summaryFilterFromQuery(q(''))).toEqual({ range: '7d', projectId: null });
    for (const r of ['90d', 'custom', '']) expect(summaryFilterFromQuery(q('range=' + r)).range).toBe('7d');
  });
  it('ignores invalid project', () => {
    expect(summaryFilterFromQuery(q('project=not-uuid')).projectId).toBeNull();
  });
  it('round-trips and writes only known keys', () => {
    const f = { range: '24h' as const, projectId: PROJECT_IDS.payments };
    expect(summaryFilterToQuery(f).toString()).toBe(`range=24h&project=${PROJECT_IDS.payments}`);
    expect(summaryFilterToQuery({ range: '7d', projectId: null }).toString()).toBe('range=7d');
    expect(summaryFilterFromQuery(summaryFilterToQuery(f))).toEqual(f);
  });
});
