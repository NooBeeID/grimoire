import { get, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import { sortGroups, type SynonymGroup } from '../types';

export type GroupListState =
  | { status: 'idle' }
  | { status: 'loading'; projectId: string; previous: SynonymGroup[] | null }
  | { status: 'ready'; projectId: string; items: SynonymGroup[] }
  | { status: 'notFound'; projectId: string }
  | { status: 'error'; projectId: string; error: ApiError; previous: SynonymGroup[] | null };

export interface GroupListStore extends Readable<GroupListState> {
  version: Readable<number>;
  begin(projectId: string): void;
  resolve(projectId: string, items: SynonymGroup[]): void;
  fail(projectId: string, e: ApiError): void;
  upsert(group: SynonymGroup): void;
  remove(id: string): void;
  invalidate(): void;
}

export function createGroupListStore(): GroupListStore {
  const state = writable<GroupListState>({ status: 'idle' });
  const version = writable(0);
  let current: string | null = null;

  const itemsOf = (s: GroupListState): SynonymGroup[] | null =>
    s.status === 'ready' ? s.items : s.status === 'loading' || s.status === 'error' ? s.previous : null;

  return {
    subscribe: state.subscribe,
    version: { subscribe: version.subscribe },
    begin(projectId) {
      current = projectId;
      state.update((s) => ({ status: 'loading', projectId, previous: itemsOf(s) }));
    },
    resolve(projectId, items) {
      if (projectId !== current) return;
      state.set({ status: 'ready', projectId, items: sortGroups(items) });
    },
    fail(projectId, e) {
      if (projectId !== current) return;
      if (e.status === 404) state.set({ status: 'notFound', projectId });
      else state.update((s) => ({ status: 'error', projectId, error: e, previous: itemsOf(s) }));
    },
    upsert(group) {
      const s = get(state);
      if (s.status !== 'ready') return;
      state.set({ ...s, items: sortGroups([...s.items.filter((g) => g.id !== group.id), group]) });
    },
    remove(id) {
      const s = get(state);
      if (s.status !== 'ready') return;
      state.set({ ...s, items: s.items.filter((g) => g.id !== id) });
    },
    invalidate() {
      version.update((n) => n + 1);
    },
  };
}
