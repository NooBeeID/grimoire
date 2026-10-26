import { describe, expect, it } from 'vitest';
import { mockGroup, mockGroupList } from './mocks/groups';
import { SynonymGroupListSchema, SynonymGroupSchema, sortGroups } from './types';

describe('SynonymGroupSchema', () => {
  it('accepts mock list', () => {
    expect(SynonymGroupListSchema.safeParse(mockGroupList()).success).toBe(true);
  });
  it('rejects a group with a single term', () => {
    expect(SynonymGroupSchema.safeParse({ ...mockGroup(1), terms: ['login'] }).success).toBe(false);
  });
  it('rejects more than 10 terms', () => {
    const terms = Array.from({ length: 11 }, (_, i) => `t${i}`);
    expect(SynonymGroupSchema.safeParse({ ...mockGroup(1), terms }).success).toBe(false);
  });
  it('rejects non-uuid id', () => {
    expect(SynonymGroupSchema.safeParse({ ...mockGroup(1), id: 'x' }).success).toBe(false);
  });
});

describe('sortGroups', () => {
  it('orders by created_at then id without mutating input', () => {
    const a = mockGroup(2, { created_at: '2026-10-24T08:00:00Z' });
    const b = mockGroup(1, { created_at: '2026-10-24T08:00:00Z' });
    const c = mockGroup(3, { created_at: '2026-10-23T08:00:00Z' });
    const input = [a, b, c];
    expect(sortGroups(input).map((g) => g.id)).toEqual([c.id, b.id, a.id]);
    expect(input[0]).toBe(a);
  });
});
