import { get } from 'svelte/store';
import { beforeEach, describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import type { User } from '../types';
import { hasFilters, offset, usersList, usersListStore as s } from './usersList';

const u = (id: string, name: string) => ({ id, name } as User);

beforeEach(() => s.reset());

describe('usersList store', () => {
  it('starts empty', () => {
    expect(get(usersList)).toMatchObject({ page: 1, data: null, loading: false, error: null });
    expect(get(hasFilters)).toBe(false);
  });
  it('setFilters resets page; offset derived', () => {
    s.setPage(3);
    expect(get(offset)).toBe(40);
    s.setFilters({ q: 'a' });
    expect(get(usersList).page).toBe(1);
    expect(get(hasFilters)).toBe(true);
  });
  it('loading lifecycle: silent keeps data', () => {
    s.succeed({ items: [u('1', 'A')], total: 1, limit: 20, offset: 0 });
    s.startLoading(true);
    expect(get(usersList)).toMatchObject({ refreshing: true, loading: false });
    expect(get(usersList).data).not.toBeNull();
    s.succeed({ items: [], total: 0, limit: 20, offset: 0 });
    expect(get(usersList).refreshing).toBe(false);
    s.startLoading(false);
    expect(get(usersList).loading).toBe(true);
    s.fail(new ApiError({ status: 500, code: 'INTERNAL', message: 'x' }));
    expect(get(usersList)).toMatchObject({ loading: false, error: { status: 500 } });
  });
  it('replaceUser swaps matching row only', () => {
    s.succeed({ items: [u('1', 'A'), u('2', 'B')], total: 2, limit: 20, offset: 0 });
    s.replaceUser(u('2', 'B2'));
    expect(get(usersList).data?.items.map((x) => x.name)).toEqual(['A', 'B2']);
  });
  it('reset restores initial', () => {
    s.setFilters({ role: 'developer' });
    s.reset();
    expect(get(hasFilters)).toBe(false);
  });
});
