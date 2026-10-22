import { get, writable, type Readable } from 'svelte/store';
import { ApiError, isApiError } from '../../../../../shared/services/http';
import { getUsageSummary } from '../services/summary.service';
import type { SummaryFilter, UsageSummary } from '../types';
import { summaryFilterKey } from './summaryQuery';

export interface SummaryState {
  status: 'idle' | 'loading' | 'ready' | 'error';
  data: UsageSummary | null;
  error: ApiError | null;
  refreshing: boolean;
  refreshError: ApiError | null;
  qualityRetrying: boolean;
  loadedAt: Date | null;
  key: string;
}

const initial = (): SummaryState => ({
  status: 'idle',
  data: null,
  error: null,
  refreshing: false,
  refreshError: null,
  qualityRetrying: false,
  loadedAt: null,
  key: '',
});

const toApi = (e: unknown) => (isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
const isAbort = (e: unknown, c: AbortController) =>
  c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError');

export interface SummaryStore extends Readable<SummaryState> {
  load(f: SummaryFilter): Promise<void>;
  reload(): Promise<void>;
  retryQuality(): Promise<void>;
  dispose(): void;
}

export function createSummaryStore(fetcher = getUsageSummary): SummaryStore {
  const store = writable<SummaryState>(initial());
  let ctrl: AbortController | null = null;
  let filter: SummaryFilter | null = null;

  function begin(): AbortController {
    ctrl?.abort();
    ctrl = new AbortController();
    return ctrl;
  }

  async function load(f: SummaryFilter) {
    const key = summaryFilterKey(f);
    filter = f;
    const c = begin();
    store.set({ ...initial(), status: 'loading', key });
    try {
      const data = await fetcher(f, c.signal);
      if (c.signal.aborted) return;
      store.update((s) => ({ ...s, status: 'ready', data, loadedAt: new Date(data.to) }));
    } catch (e) {
      if (isAbort(e, c)) return;
      store.update((s) => ({ ...s, status: 'error', data: null, error: toApi(e) }));
    }
  }

  async function reload() {
    const f = filter;
    const s = get(store);
    if (!f || s.status === 'idle') return;
    if (s.status === 'error') return load(f);
    const c = begin();
    store.update((x) => ({ ...x, refreshing: true, refreshError: null }));
    try {
      const data = await fetcher(f, c.signal);
      if (c.signal.aborted) return;
      store.update((x) => ({ ...x, data, refreshing: false, loadedAt: new Date(data.to) }));
    } catch (e) {
      if (isAbort(e, c)) return;
      store.update((x) => ({ ...x, refreshing: false, refreshError: toApi(e) }));
    }
  }

  async function retryQuality() {
    const f = filter;
    if (!f) return;
    const c = begin();
    store.update((x) => ({ ...x, qualityRetrying: true }));
    try {
      const data = await fetcher(f, c.signal);
      if (c.signal.aborted) return;
      store.update((x) => ({ ...x, data, qualityRetrying: false, loadedAt: new Date(data.to) }));
    } catch (e) {
      if (isAbort(e, c)) return;
      store.update((x) => ({ ...x, qualityRetrying: false }));
    }
  }

  return { subscribe: store.subscribe, load, reload, retryQuality, dispose: () => ctrl?.abort() };
}
