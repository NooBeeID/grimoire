import { writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { ProjectList } from '../types';

export type ListState =
  | { status: 'idle' }
  | { status: 'loading'; key: string; previous: ProjectList | null }
  | { status: 'ready'; key: string; data: ProjectList }
  | { status: 'error'; key: string; error: ApiError };

export function createProjectListStore(): Readable<ListState> & {
  begin(key: string): void;
  resolve(key: string, d: ProjectList): void;
  fail(key: string, e: ApiError): void;
} {
  const state = writable<ListState>({ status: 'idle' });
  let current: ListState = { status: 'idle' };
  const set = (s: ListState) => {
    current = s;
    state.set(s);
  };
  return {
    subscribe: state.subscribe,
    begin(key) {
      const previous = current.status === 'ready' ? current.data : current.status === 'loading' ? current.previous : null;
      set({ status: 'loading', key, previous });
    },
    resolve(key, data) {
      if (current.status !== 'loading' || current.key !== key) return;
      set({ status: 'ready', key, data });
    },
    fail(key, error) {
      if (current.status !== 'loading' || current.key !== key) return;
      set({ status: 'error', key, error });
    },
  };
}
