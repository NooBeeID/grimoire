import { describe, expect, it } from 'vitest';
import { mockKey, mockKeyList, mockRevokedKey } from './mocks/keys';
import { ApiKeyListSchema, displayPrefix, sortKeys } from './types';

describe('ApiKeyListSchema', () => {
  it('accepts a valid list incl. null last_used_at', () => {
    expect(ApiKeyListSchema.safeParse(mockKeyList(mockKey(1), mockRevokedKey(2))).success).toBe(true);
  });
  it('rejects bad status, bad prefix, missing items', () => {
    expect(ApiKeyListSchema.safeParse(mockKeyList({ ...mockKey(1), status: 'disabled' } as never)).success).toBe(false);
    expect(ApiKeyListSchema.safeParse(mockKeyList(mockKey(1, { prefix: 'ABCDEFGH' }))).success).toBe(false);
    expect(ApiKeyListSchema.safeParse({}).success).toBe(false);
  });
});

describe('sortKeys', () => {
  it('active first, created_at desc, id asc; does not mutate', () => {
    const a = mockKey(1, { created_at: '2026-10-23T08:00:00Z' });
    const b = mockKey(2, { created_at: '2026-10-23T10:00:00Z' });
    const c = mockKey(3, { created_at: '2026-10-23T10:00:00Z' });
    const r = mockRevokedKey(4, { created_at: '2026-10-24T10:00:00Z' });
    const input = [r, a, c, b];
    expect(sortKeys(input).map((k) => k.id)).toEqual([b.id, c.id, a.id, r.id]);
    expect(input[0]).toBe(r);
  });
  it('displayPrefix', () => expect(displayPrefix('k3x9q2ma')).toBe('gr_k3x9q2ma_…'));
});
