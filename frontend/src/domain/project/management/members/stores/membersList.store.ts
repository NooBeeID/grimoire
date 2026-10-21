import { derived, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { Member } from '../types';

export interface MembersState {
  status: 'idle' | 'loading' | 'ready' | 'error';
  items: Member[];
  error: ApiError | null;
}

// Mengikuti urutan backend: owner dulu, nama (lowercase; null di akhir grup), lalu user_id.
export function sortMembers(items: Member[]): Member[] {
  return [...items].sort((a, b) => {
    if (a.role !== b.role) return a.role === 'owner' ? -1 : 1;
    const an = a.name?.toLowerCase() ?? null;
    const bn = b.name?.toLowerCase() ?? null;
    if (an !== bn) {
      if (an === null) return 1;
      if (bn === null) return -1;
      return an < bn ? -1 : 1;
    }
    return a.user_id < b.user_id ? -1 : a.user_id > b.user_id ? 1 : 0;
  });
}

export function createMembersListStore() {
  const state = writable<MembersState>({ status: 'idle', items: [], error: null });
  const ownerCount: Readable<number> = derived(state, ($s) => $s.items.filter((m) => m.role === 'owner').length);
  return {
    subscribe: state.subscribe,
    ownerCount,
    begin() {
      state.update((s) => ({ ...s, status: s.status === 'ready' ? 'ready' : 'loading', error: null }));
    },
    resolve(items: Member[]) {
      state.set({ status: 'ready', items: sortMembers(items), error: null });
    },
    fail(error: ApiError) {
      state.set({ status: 'error', items: [], error });
    },
    upsert(m: Member) {
      state.update((s) => ({
        ...s,
        items: sortMembers([...s.items.filter((x) => x.user_id !== m.user_id), m]),
      }));
    },
    remove(userId: string) {
      state.update((s) => ({ ...s, items: s.items.filter((x) => x.user_id !== userId) }));
    },
  };
}
export type MembersListStore = ReturnType<typeof createMembersListStore>;
