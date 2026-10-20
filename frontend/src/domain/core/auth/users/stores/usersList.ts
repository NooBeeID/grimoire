import { derived, get, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import { PAGE_SIZE, type User, type UserList, type UsersFilters } from '../types';

export interface UsersListState {
  filters: UsersFilters;
  page: number;
  data: UserList | null;
  loading: boolean;
  refreshing: boolean;
  error: ApiError | null;
}

const emptyFilters = (): UsersFilters => ({ role: '', status: '', q: '' });
const initial = (): UsersListState => ({ filters: emptyFilters(), page: 1, data: null, loading: false, refreshing: false, error: null });

const store = writable<UsersListState>(initial());

export const usersList: Readable<UsersListState> = { subscribe: store.subscribe };
export const offset = derived(store, (s) => (s.page - 1) * PAGE_SIZE);
export const hasFilters = derived(store, (s) => s.filters.role !== '' || s.filters.status !== '' || s.filters.q.trim() !== '');

export const usersListStore = {
  state: () => get(store),
  setFilters(partial: Partial<UsersFilters>): void {
    store.update((s) => ({ ...s, filters: { ...s.filters, ...partial }, page: 1 }));
  },
  setPage(n: number): void {
    store.update((s) => ({ ...s, page: Math.max(1, n) }));
  },
  startLoading(silent: boolean): void {
    store.update((s) => (silent ? { ...s, refreshing: true } : { ...s, loading: true, error: null }));
  },
  succeed(data: UserList): void {
    store.update((s) => ({ ...s, data, loading: false, refreshing: false, error: null }));
  },
  fail(error: ApiError): void {
    store.update((s) => ({ ...s, loading: false, refreshing: false, error }));
  },
  replaceUser(user: User): void {
    store.update((s) =>
      s.data ? { ...s, data: { ...s.data, items: s.data.items.map((u) => (u.id === user.id ? user : u)) } } : s,
    );
  },
  reset(): void {
    store.set(initial());
  },
};
export type UsersListStore = typeof usersListStore;
export const resetUsersState = usersListStore.reset;
