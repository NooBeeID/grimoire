import { get, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import { sortKeys, type ApiKey } from '../types';

export type KeyListState =
  | { status: 'idle' }
  | { status: 'loading'; projectId: string; previous: ApiKey[] | null }
  | { status: 'ready'; projectId: string; items: ApiKey[] }
  | { status: 'notFound'; projectId: string }
  | { status: 'error'; projectId: string; error: ApiError; previous: ApiKey[] | null };

export interface KeyListStore extends Readable<KeyListState> {
  version: Readable<number>;
  begin(projectId: string): void;
  resolve(projectId: string, items: ApiKey[]): void;
  fail(projectId: string, e: ApiError): void;
  upsert(key: ApiKey): void;
  invalidate(): void;
}

export const activeCount = (s: KeyListState): number =>
  s.status === 'ready' ? s.items.filter((k) => k.status === 'active').length : 0;

export function createKeyListStore(): KeyListStore {
  const state = writable<KeyListState>({ status: 'idle' });
  const version = writable(0);
  let current: string | null = null;

  const itemsOf = (s: KeyListState): ApiKey[] | null =>
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
      state.set({ status: 'ready', projectId, items: sortKeys(items) });
    },
    fail(projectId, e) {
      if (projectId !== current) return;
      if (e.status === 404) state.set({ status: 'notFound', projectId });
      else state.update((s) => ({ status: 'error', projectId, error: e, previous: itemsOf(s) }));
    },
    upsert(key) {
      const s = get(state);
      if (s.status !== 'ready') return;
      const rest = s.items.filter((k) => k.id !== key.id);
      state.set({ ...s, items: sortKeys([...rest, key]) });
    },
    invalidate() {
      version.update((n) => n + 1);
    },
  };
}
