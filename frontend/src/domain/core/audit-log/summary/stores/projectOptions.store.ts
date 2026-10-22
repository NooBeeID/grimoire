import { writable, type Readable } from 'svelte/store';
import { ApiError, isApiError } from '../../../../../shared/services/http';
import { listProjectOptions } from '../services/projectOptions.service';
import type { ProjectOption } from '../types';

export interface ProjectOptionsState {
  status: 'idle' | 'loading' | 'ready' | 'error';
  options: ProjectOption[];
  total: number;
  error: ApiError | null;
}

export interface ProjectOptionsStore extends Readable<ProjectOptionsState> {
  load(): Promise<void>;
  retry(): Promise<void>;
  dispose(): void;
}

export function createProjectOptionsStore(fetcher = listProjectOptions): ProjectOptionsStore {
  const store = writable<ProjectOptionsState>({ status: 'idle', options: [], total: 0, error: null });
  let ctrl: AbortController | null = null;

  async function load() {
    ctrl?.abort();
    const c = (ctrl = new AbortController());
    store.update((s) => ({ ...s, status: 'loading', error: null }));
    try {
      const r = await fetcher(c.signal);
      if (c.signal.aborted) return;
      store.set({ status: 'ready', options: r.options, total: r.total, error: null });
    } catch (e) {
      if (c.signal.aborted) return;
      const error = isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) });
      store.set({ status: 'error', options: [], total: 0, error });
    }
  }

  return { subscribe: store.subscribe, load, retry: load, dispose: () => ctrl?.abort() };
}
