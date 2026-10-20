import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { usersList, usersListStore as store } from '../stores/usersList';
import type { UserList } from '../types';
import { createUsersListController } from './usersList.controller';

const list = (total: number, n = 0, offset = 0): UserList => ({
  items: Array.from({ length: n }, (_, i) => ({ id: `${i}` }) as never),
  total, limit: 20, offset,
});

beforeEach(() => store.reset());

describe('usersList controller', () => {
  it('loads first page and stores data', async () => {
    const listUsers = vi.fn().mockResolvedValue(list(1, 1));
    const c = createUsersListController({ listUsers, store });
    await c.load();
    expect(listUsers.mock.calls[0]![0]).toMatchObject({ limit: 20, offset: 0 });
    expect(get(usersList).data?.total).toBe(1);
  });
  it('debounces q; other filters load immediately', async () => {
    vi.useFakeTimers();
    const listUsers = vi.fn().mockResolvedValue(list(0));
    const c = createUsersListController({ listUsers, store });
    void c.setFilters({ q: 'a' });
    void c.setFilters({ q: 'ab' });
    expect(listUsers).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(300);
    expect(listUsers).toHaveBeenCalledTimes(1);
    expect(listUsers.mock.calls[0]![0].q).toBe('ab');
    await c.setFilters({ role: 'developer' });
    expect(listUsers).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });
  it('ignores stale responses and aborts previous request', async () => {
    const resolvers: ((v: UserList) => void)[] = [];
    const signals: AbortSignal[] = [];
    const listUsers = vi.fn((_p, s?: AbortSignal) => {
      signals.push(s!);
      return new Promise<UserList>((r) => resolvers.push(r));
    });
    const c = createUsersListController({ listUsers, store });
    const a = c.load();
    const b = c.load();
    expect(signals[0]!.aborted).toBe(true);
    resolvers[1]!(list(5, 5));
    resolvers[0]!(list(99, 1));
    await Promise.all([a, b]);
    expect(get(usersList).data?.total).toBe(5);
  });
  it('AbortError is not an error; ApiError is stored', async () => {
    const c1 = createUsersListController({ listUsers: vi.fn().mockRejectedValue(new DOMException('x', 'AbortError')), store });
    await c1.load();
    expect(get(usersList).error).toBeNull();
    const c2 = createUsersListController({ listUsers: vi.fn().mockRejectedValue(new ApiError({ status: 500, code: 'INTERNAL', message: 'x' })), store });
    await c2.load();
    expect(get(usersList).error?.status).toBe(500);
  });
  it('jumps once to last valid page when page is empty', async () => {
    const listUsers = vi.fn(async (p: { offset: number }) => (p.offset >= 40 ? list(25, 0, p.offset) : list(25, 5, p.offset)));
    store.setPage(5);
    const c = createUsersListController({ listUsers, store });
    await c.load();
    expect(get(usersList).page).toBe(2);
    expect(listUsers).toHaveBeenCalledTimes(2);
    expect(get(usersList).data?.items).toHaveLength(5);
  });
  it('does not loop when still empty after clamping', async () => {
    const listUsers = vi.fn(async () => list(25, 0));
    store.setPage(5);
    await createUsersListController({ listUsers, store }).load();
    expect(listUsers).toHaveBeenCalledTimes(2);
  });
  it('silent refresh keeps old data visible; dispose cancels timer', async () => {
    vi.useFakeTimers();
    const listUsers = vi.fn().mockResolvedValue(list(1, 1));
    const c = createUsersListController({ listUsers, store });
    await c.load();
    const p = c.refresh({ silent: true });
    expect(get(usersList)).toMatchObject({ refreshing: true, loading: false });
    await p;
    void c.setFilters({ q: 'zz' });
    c.dispose();
    await vi.advanceTimersByTimeAsync(500);
    expect(listUsers).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });
});
