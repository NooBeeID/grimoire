import { writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { ProjectDetail } from '../types';

export type DetailState =
  | { status: 'idle' }
  | { status: 'loading'; id: string; previous: ProjectDetail | null }
  | { status: 'ready'; id: string; project: ProjectDetail }
  | { status: 'notFound'; id: string }
  | { status: 'error'; id: string; error: ApiError };

export function createProjectDetailStore(): Readable<DetailState> & {
  begin(id: string): void;
  resolve(id: string, p: ProjectDetail): void;
  fail(id: string, e: ApiError): void;
  replace(p: ProjectDetail): void;
} {
  const state = writable<DetailState>({ status: 'idle' });
  let cur: DetailState = { status: 'idle' };
  const set = (s: DetailState) => {
    cur = s;
    state.set(s);
  };
  return {
    subscribe: state.subscribe,
    begin(id) {
      const previous =
        cur.status === 'ready' && cur.id === id ? cur.project : cur.status === 'loading' && cur.id === id ? cur.previous : null;
      set({ status: 'loading', id, previous });
    },
    resolve(id, project) {
      if (cur.status !== 'loading' || cur.id !== id) return;
      set({ status: 'ready', id, project });
    },
    fail(id, error) {
      if (cur.status !== 'loading' || cur.id !== id) return;
      set(error.status === 404 ? { status: 'notFound', id } : { status: 'error', id, error });
    },
    replace(project) {
      if (cur.status !== 'ready' || cur.id !== project.id) return;
      set({ status: 'ready', id: cur.id, project });
    },
  };
}
