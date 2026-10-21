import { get, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import { sortSpecs, type ApiSpec } from '../types';

export type SpecListState =
  | { status: 'idle' }
  | { status: 'loading'; projectId: string; previous: ApiSpec[] | null; total: number; limit: number }
  | { status: 'ready'; projectId: string; items: ApiSpec[]; total: number; limit: number }
  | { status: 'notFound'; projectId: string }
  | { status: 'error'; projectId: string; error: ApiError; previous: ApiSpec[] | null };

export interface SpecListStore extends Readable<SpecListState> {
  version: Readable<number>;
  begin(projectId: string): void;
  resolve(projectId: string, items: ApiSpec[], total: number, limit: number): void;
  fail(projectId: string, e: ApiError): void;
  upsert(spec: ApiSpec): void;
  rename(oldName: string, spec: ApiSpec): void;
  remove(name: string): void;
  invalidate(): void;
  names(): Set<string>;
}

export function createSpecListStore(): SpecListStore {
  const state = writable<SpecListState>({ status: 'idle' });
  const version = writable(0);
  let current: string | null = null;

  const itemsOf = (s: SpecListState): ApiSpec[] | null =>
    s.status === 'ready' ? s.items : s.status === 'loading' || s.status === 'error' ? s.previous : null;

  return {
    subscribe: state.subscribe,
    version: { subscribe: version.subscribe },
    begin(projectId) {
      current = projectId;
      state.update((s) => ({
        status: 'loading',
        projectId,
        previous: itemsOf(s),
        total: s.status === 'ready' || s.status === 'loading' ? s.total : 0,
        limit: s.status === 'ready' || s.status === 'loading' ? s.limit : 0,
      }));
    },
    resolve(projectId, items, total, limit) {
      if (projectId !== current) return;
      state.set({ status: 'ready', projectId, items: sortSpecs(items), total, limit });
    },
    fail(projectId, e) {
      if (projectId !== current) return;
      if (e.status === 404) state.set({ status: 'notFound', projectId });
      else state.update((s) => ({ status: 'error', projectId, error: e, previous: itemsOf(s) }));
    },
    upsert(spec) {
      const s = get(state);
      if (s.status !== 'ready') return;
      const exists = s.items.some((x) => x.name === spec.name);
      const rest = s.items.filter((x) => x.name !== spec.name);
      state.set({ ...s, items: sortSpecs([...rest, spec]), total: exists ? s.total : s.total + 1 });
    },
    rename(oldName, spec) {
      const s = get(state);
      if (s.status !== 'ready') return;
      const rest = s.items.filter((x) => x.name !== oldName && x.name !== spec.name);
      state.set({ ...s, items: sortSpecs([...rest, spec]), total: rest.length + 1 });
    },
    remove(name) {
      const s = get(state);
      if (s.status !== 'ready') return;
      const items = s.items.filter((x) => x.name !== name);
      state.set({ ...s, items, total: Math.max(0, s.total - (s.items.length - items.length)) });
    },
    invalidate() {
      version.update((n) => n + 1);
    },
    names() {
      const s = get(state);
      return new Set(itemsOf(s)?.map((x) => x.name) ?? []);
    },
  };
}
