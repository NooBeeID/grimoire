import { get, writable, type Readable } from 'svelte/store';
import { ApiError, isApiError } from '../../../../../shared/services/http';
import { getAgentUsage } from '../services/usage.service';
import {
  PROJECT_SECTIONS,
  SECTION_KEY,
  USAGE_SECTIONS,
  type FileRead,
  type KeyUsage,
  type UnreadFile,
  type UsageFilter,
  type UsageSection,
  type ZeroQuery,
} from '../types';
import { usageFilterKey } from './usageQuery';

export type SectionStatus = 'loading' | 'ready' | 'empty' | 'error' | 'needs_project';
export interface SectionState<T> {
  status: SectionStatus;
  items: T[];
  truncated: boolean;
  error: ApiError | 'unavailable' | null;
}
export interface UsageState {
  key: string;
  top_files: SectionState<FileRead>;
  unread_files: SectionState<UnreadFile>;
  zero_queries: SectionState<ZeroQuery>;
  keys: SectionState<KeyUsage>;
}
export interface UsageStore extends Readable<UsageState> {
  load(f: UsageFilter): void;
  retry(s: UsageSection): Promise<void>;
  dispose(): void;
}

const section = <T>(status: SectionStatus): SectionState<T> => ({ status, items: [], truncated: false, error: null });
const initial = (): UsageState => ({
  key: '',
  top_files: section('loading'),
  unread_files: section('loading'),
  zero_queries: section('loading'),
  keys: section('loading'),
});

const toApi = (e: unknown) => (isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) }));
const isAbort = (e: unknown, c: AbortController) =>
  c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError');

export function createUsageStore(fetcher = getAgentUsage): UsageStore {
  const store = writable<UsageState>(initial());
  const ctrls = new Map<UsageSection, AbortController>();
  let filter: UsageFilter | null = null;

  const setSection = (s: UsageSection, v: SectionState<never> | SectionState<unknown>) =>
    store.update((x) => ({ ...x, [s]: v }));

  async function loadSection(s: UsageSection, f: UsageFilter, key: string) {
    ctrls.get(s)?.abort();
    const c = new AbortController();
    ctrls.set(s, c);
    setSection(s, section('loading'));
    try {
      const res = await fetcher(f, s, c.signal);
      if (c.signal.aborted || get(store).key !== key) return;
      const body = res[SECTION_KEY[s]] as {
        available: boolean;
        error_code?: string;
        items: unknown[];
        truncated?: boolean;
      } | null;
      if (!body) return setSection(s, section('empty'));
      if (!body.available) {
        return setSection(
          s,
          body.error_code === 'PROJECT_REQUIRED'
            ? section('needs_project')
            : { ...section('error'), error: 'unavailable' },
        );
      }
      setSection(s, {
        status: body.items.length === 0 ? 'empty' : 'ready',
        items: body.items,
        truncated: body.truncated ?? false,
        error: null,
      });
    } catch (e) {
      if (isAbort(e, c) || get(store).key !== key) return;
      setSection(s, { ...section('error'), error: toApi(e) });
    }
  }

  function load(f: UsageFilter) {
    filter = f;
    const key = usageFilterKey(f);
    ctrls.forEach((c) => c.abort());
    ctrls.clear();
    store.set({ ...initial(), key });
    for (const s of USAGE_SECTIONS) {
      if (!f.projectId && PROJECT_SECTIONS.includes(s)) setSection(s, section('needs_project'));
      else void loadSection(s, f, key);
    }
  }

  async function retry(s: UsageSection) {
    if (!filter) return;
    await loadSection(s, filter, usageFilterKey(filter));
  }

  return {
    subscribe: store.subscribe,
    load,
    retry,
    dispose: () => ctrls.forEach((c) => c.abort()),
  };
}
