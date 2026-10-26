import { describe, expect, it } from 'vitest';
import { usageFilterFromQuery, usageFilterKey } from './usageQuery';

describe('usageFilterFromQuery', () => {
  it('defaults invalid range to 7d and drops non-uuid project', () => {
    expect(usageFilterFromQuery(new URLSearchParams('range=1y&project=abc'))).toEqual({ range: '7d', projectId: null });
  });
  it('keeps valid range and project', () => {
    const id = '0192f5a0-1111-7aaa-8bbb-000000000001';
    const f = usageFilterFromQuery(new URLSearchParams(`range=30d&project=${id}`));
    expect(f).toEqual({ range: '30d', projectId: id });
    expect(usageFilterKey(f)).toBe(`30d|${id}`);
  });
});
