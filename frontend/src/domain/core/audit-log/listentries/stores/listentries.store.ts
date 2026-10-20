import { derived, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { ResolvedRange } from '../services/listentries.service';
import type { ActorType, AuditSummary } from '../types';

export interface PageState {
  cursor: string | null;
  items: AuditSummary[];
  nextCursor: string | null;
}
export interface ListState {
  key: string;
  range: ResolvedRange;
  pages: PageState[];
  status: 'idle' | 'loading' | 'ready' | 'error';
  error: ApiError | null;
  moreStatus: 'idle' | 'loading' | 'error';
  moreError: ApiError | null;
}
export interface Suggestions {
  actors: { actorType: ActorType; actorId: string | null; actorLabel: string }[];
  projects: { projectId: string; projectLabel: string | null }[];
}

const MAX_SUGGESTIONS = 50;
const initial = (): ListState => ({
  key: '',
  range: {},
  pages: [],
  status: 'idle',
  error: null,
  moreStatus: 'idle',
  moreError: null,
});

export function dedupeRows(pages: PageState[]): AuditSummary[] {
  const seen = new Set<string>();
  const rows: AuditSummary[] = [];
  for (const p of pages)
    for (const r of p.items) {
      if (seen.has(r.id)) continue;
      seen.add(r.id);
      rows.push(r);
    }
  return rows;
}

export function computeSuggestions(rows: AuditSummary[]): Suggestions {
  const actors = new Map<string, Suggestions['actors'][number]>();
  const projects = new Map<string, Suggestions['projects'][number]>();
  for (const r of rows) {
    const ak = `${r.actor_type}|${r.actor_id ?? ''}|${r.actor_label}`;
    if (!actors.has(ak) && actors.size < MAX_SUGGESTIONS) {
      actors.set(ak, { actorType: r.actor_type, actorId: r.actor_id, actorLabel: r.actor_label });
    }
    if (r.project_id && !projects.has(r.project_id) && projects.size < MAX_SUGGESTIONS) {
      projects.set(r.project_id, { projectId: r.project_id, projectLabel: r.project_label });
    }
  }
  return { actors: [...actors.values()], projects: [...projects.values()] };
}

export function createListEntriesStore() {
  const state = writable<ListState>(initial());
  const rows: Readable<AuditSummary[]> = derived(state, ($s) => dedupeRows($s.pages));
  const nextCursor: Readable<string | null> = derived(state, ($s) => $s.pages.at(-1)?.nextCursor ?? null);
  const hasMore = derived(nextCursor, ($c) => $c !== null);
  const suggestions = derived(rows, computeSuggestions);
  return {
    subscribe: state.subscribe,
    rows,
    nextCursor,
    hasMore,
    suggestions,
    reset(key: string, range: ResolvedRange) {
      state.set({ ...initial(), key, range });
    },
    begin() {
      state.update((s) => ({ ...s, status: 'loading', error: null }));
    },
    pushPage(page: PageState) {
      state.update((s) => ({
        ...s,
        pages: [...s.pages, page],
        status: 'ready',
        error: null,
        moreStatus: 'idle',
        moreError: null,
      }));
    },
    fail(err: ApiError) {
      state.update((s) => ({ ...s, status: 'error', error: err }));
    },
    beginMore() {
      state.update((s) => ({ ...s, moreStatus: 'loading', moreError: null }));
    },
    failMore(err: ApiError) {
      state.update((s) => ({ ...s, moreStatus: 'error', moreError: err }));
    },
  };
}
export type ListEntriesStore = ReturnType<typeof createListEntriesStore>;
