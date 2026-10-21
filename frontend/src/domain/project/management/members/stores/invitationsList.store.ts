import { writable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import { sortInvitations } from '../model/invite';
import type { Invitation } from '../types';

export interface InvitationsState {
  status: 'idle' | 'loading' | 'ready' | 'error';
  items: Invitation[];
  error: ApiError | null;
  refreshing: boolean;
}

const initial = (): InvitationsState => ({ status: 'idle', items: [], error: null, refreshing: false });

export function createInvitationsListStore() {
  const state = writable<InvitationsState>(initial());
  return {
    subscribe: state.subscribe,
    begin() {
      state.update((s) => ({ ...s, status: s.status === 'ready' ? 'ready' : 'loading', error: null }));
    },
    // Item lama tetap tampil selama muat ulang diam.
    refresh() {
      state.update((s) => ({ ...s, refreshing: true }));
    },
    resolve(items: Invitation[]) {
      state.set({ status: 'ready', items: sortInvitations(items), error: null, refreshing: false });
    },
    fail(error: ApiError) {
      state.set({ status: 'error', items: [], error, refreshing: false });
    },
    remove(userId: string) {
      state.update((s) => ({ ...s, items: s.items.filter((x) => x.user_id !== userId) }));
    },
    clear() {
      state.set(initial());
    },
  };
}
export type InvitationsListStore = ReturnType<typeof createInvitationsListStore>;
