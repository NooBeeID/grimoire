import { writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import { toDetailError } from '../services/getentry.service';
import type { AuditEntryDetail, DetailState } from '../types';

export function createEntryDetailStore(): Readable<DetailState> & {
  begin(id: string): void;
  resolve(id: string, e: AuditEntryDetail): void;
  fail(id: string, err: ApiError): void;
  reset(): void;
} {
  const store = writable<DetailState>({ status: 'idle' });
  let currentId: string | null = null;
  return {
    subscribe: store.subscribe,
    begin(id) {
      currentId = id;
      store.set({ status: 'loading', id });
    },
    resolve(id, entry) {
      if (id !== currentId) return;
      store.set({ status: 'ready', id, entry });
    },
    fail(id, apiError) {
      if (id !== currentId) return;
      store.set({ status: 'error', id, error: toDetailError(apiError), apiError });
    },
    reset() {
      currentId = null;
      store.set({ status: 'idle' });
    },
  };
}
