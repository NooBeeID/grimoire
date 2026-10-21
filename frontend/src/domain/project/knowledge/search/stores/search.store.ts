import { writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { SearchResult } from '../types';

export type SearchState =
  | { status: 'idle' }
  | { status: 'loading'; key: string; previous: SearchResult | null }
  | { status: 'ready'; key: string; result: SearchResult }
  | { status: 'error'; key: string; error: ApiError };

export interface SearchStore extends Readable<SearchState> {
  begin(key: string): void;
  resolve(key: string, r: SearchResult): void;
  fail(key: string, e: ApiError): void;
  reset(): void;
}

export function createSearchStore(): SearchStore {
  const s = writable<SearchState>({ status: 'idle' });
  let latest = '';
  let last: SearchResult | null = null;
  return {
    subscribe: s.subscribe,
    begin(key) {
      latest = key;
      s.set({ status: 'loading', key, previous: last });
    },
    resolve(key, result) {
      if (key !== latest) return;
      last = result;
      s.set({ status: 'ready', key, result });
    },
    fail(key, error) {
      if (key !== latest) return;
      s.set({ status: 'error', key, error });
    },
    reset() {
      latest = '';
      last = null;
      s.set({ status: 'idle' });
    },
  };
}
