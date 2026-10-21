import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { mockMember } from '../mocks/members';
import { createMembersListStore, sortMembers } from './membersList.store';

describe('sortMembers', () => {
  it('owner first, name case-insensitive, null name last in group, user_id tie-break', () => {
    const items = [
      mockMember(5, { name: 'zed' }),
      mockMember(4, { name: null, email: null }),
      mockMember(3, { name: 'Andi' }),
      mockMember(2, { name: 'andi' }),
      mockMember(1, { name: 'Bob', role: 'owner' }),
    ];
    const r = sortMembers(items).map((m) => m.user_id.slice(-1));
    expect(r).toEqual(['1', '2', '3', '5', '4']);
  });
});

describe('membersList store', () => {
  it('upsert reorders and ownerCount is derived', () => {
    const s = createMembersListStore();
    s.resolve([mockMember(2, { name: 'B' }), mockMember(3, { name: 'C' })]);
    expect(get(s.ownerCount)).toBe(0);
    s.upsert(mockMember(3, { name: 'C', role: 'owner' }));
    expect(get(s).items[0]!.name).toBe('C');
    expect(get(s.ownerCount)).toBe(1);
    s.remove(get(s).items[0]!.user_id);
    expect(get(s).items).toHaveLength(1);
  });
  it('fail clears items', () => {
    const s = createMembersListStore();
    s.resolve([mockMember(1)]);
    s.fail({ status: 500 } as never);
    expect(get(s).status).toBe('error');
    expect(get(s).items).toEqual([]);
  });
});
