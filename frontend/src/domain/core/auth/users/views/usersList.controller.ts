import { isApiError } from '../../../../../shared/services/http';
import type { ListUsersParams } from '../services/users.service';
import type { UsersListStore } from '../stores/usersList';
import { PAGE_SIZE, type UserList, type UsersFilters } from '../types';

export interface ControllerDeps {
  listUsers: (p: ListUsersParams, signal?: AbortSignal) => Promise<UserList>;
  store: UsersListStore;
  debounceMs?: number;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (h: unknown) => void;
}

export function createUsersListController(deps: ControllerDeps) {
  const { listUsers, store, debounceMs = 300 } = deps;
  const setTimer = deps.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
  const clearTimer = deps.clearTimer ?? ((h) => clearTimeout(h as ReturnType<typeof setTimeout>));
  let abort: AbortController | null = null;
  let seq = 0;
  let timer: unknown = null;

  function cancelTimer() {
    if (timer !== null) clearTimer(timer);
    timer = null;
  }

  async function fetchPage(silent: boolean, allowClamp: boolean): Promise<void> {
    cancelTimer();
    abort?.abort();
    const ctl = new AbortController();
    abort = ctl;
    const mine = ++seq;
    store.startLoading(silent);
    const st = store.state();
    try {
      const data = await listUsers({ ...st.filters, limit: PAGE_SIZE, offset: (st.page - 1) * PAGE_SIZE }, ctl.signal);
      if (mine !== seq) return;
      if (allowClamp && data.total > 0 && data.items.length === 0 && st.page > 1) {
        store.setPage(Math.ceil(data.total / PAGE_SIZE));
        await fetchPage(silent, false);
        return;
      }
      store.succeed(data);
    } catch (e) {
      if (mine !== seq) return;
      if ((e instanceof DOMException && e.name === 'AbortError') || ctl.signal.aborted) return;
      if (isApiError(e)) store.fail(e);
      else throw e;
    }
  }

  return {
    load: () => fetchPage(false, true),
    refresh: (o: { silent?: boolean } = {}) => fetchPage(o.silent ?? false, true),
    setFilters(partial: Partial<UsersFilters>): Promise<void> | void {
      const qOnly = Object.keys(partial).length === 1 && 'q' in partial;
      store.setFilters(partial);
      if (qOnly) {
        cancelTimer();
        timer = setTimer(() => void fetchPage(false, true), debounceMs);
        return;
      }
      return fetchPage(false, true);
    },
    setPage(n: number): Promise<void> {
      store.setPage(n);
      return fetchPage(false, true);
    },
    dispose() {
      cancelTimer();
      seq++;
      abort?.abort();
      abort = null;
    },
  };
}
export type UsersListController = ReturnType<typeof createUsersListController>;
