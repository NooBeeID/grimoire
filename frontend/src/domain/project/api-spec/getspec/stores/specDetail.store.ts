import { derived, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import { buildTagTree, type TagGroup } from '../model/operations';
import { filterEndpoints } from '../model/search';
import type { ApiSpec, ApiSpecDetail, ApiSpecEndpoint } from '../types';

export type SpecDetailState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ready'; detail: ApiSpecDetail }
  | { status: 'error'; error: ApiError };

export interface SpecDetailStore extends Readable<SpecDetailState> {
  query: Readable<string>;
  tree: Readable<TagGroup[]>;
  matches: Readable<ApiSpecEndpoint[]>;
  start(): void;
  set(detail: ApiSpecDetail): void;
  fail(e: ApiError): void;
  setQuery(q: string): void;
  replaceSpec(spec: ApiSpec): void;
}

export function createSpecDetailStore(): SpecDetailStore {
  const state = writable<SpecDetailState>({ status: 'idle' });
  const query = writable('');
  const tree = derived(state, (s) => (s.status === 'ready' ? buildTagTree(s.detail.document, s.detail.endpoints) : []));
  const matches = derived([state, query], ([s, q]) => (s.status === 'ready' ? filterEndpoints(s.detail.endpoints, q) : []));
  return {
    subscribe: state.subscribe,
    query: { subscribe: query.subscribe },
    tree,
    matches,
    start: () => state.set({ status: 'loading' }),
    set: (detail) => state.set({ status: 'ready', detail }),
    fail: (error) => state.set({ status: 'error', error }),
    setQuery: (q) => query.set(q),
    replaceSpec: (spec) => state.update((s) => (s.status === 'ready' ? { status: 'ready', detail: { ...s.detail, spec } } : s)),
  };
}
