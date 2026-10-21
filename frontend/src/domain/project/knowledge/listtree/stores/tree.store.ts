import { derived, get, writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { TreeLevel, TreeRow } from '../types';

export interface LevelState {
  status: 'loading' | 'ready' | 'error';
  level?: TreeLevel;
  error?: ApiError;
}
interface State {
  levels: Map<string, LevelState>;
  expanded: Set<string>;
}

export interface TreeStore {
  state: Readable<State>;
  rows: Readable<TreeRow[]>;
  root: Readable<LevelState | undefined>;
  has(prefix: string): boolean;
  beginLoad(prefix: string): void;
  resolve(prefix: string, level: TreeLevel): void;
  fail(prefix: string, error: ApiError): void;
  toggle(prefix: string): boolean;
  expand(prefixes: string[]): void;
  isExpanded(prefix: string): boolean;
  invalidateAll(): string[];
}

export function buildRows(levels: Map<string, LevelState>, expanded: Set<string>): TreeRow[] {
  const out: TreeRow[] = [];
  const walk = (prefix: string, depth: number) => {
    const ls = levels.get(prefix);
    if (!ls) return;
    if (!ls.level) {
      const kind = ls.status === 'error' ? 'error' : 'loading';
      out.push({ key: `${kind}:${prefix}`, kind, path: prefix, name: '', depth, parent: prefix });
      return;
    }
    for (const f of ls.level.folders) {
      const open = expanded.has(f.path);
      out.push({ key: `d:${f.path}`, kind: 'folder', path: f.path, name: f.name, depth, expanded: open, fileCount: f.file_count, parent: prefix });
      if (open) {
        const child = levels.get(f.path);
        if (child?.level && child.level.folders.length + child.level.files.length === 0) {
          out.push({ key: `empty:${f.path}`, kind: 'empty', path: f.path, name: '', depth: depth + 1, parent: f.path });
        } else walk(f.path, depth + 1);
      }
    }
    for (const f of ls.level.files) {
      out.push({ key: `f:${f.path}`, kind: 'file', path: f.path, name: f.name, depth, parent: prefix });
    }
  };
  walk('', 0);
  return out;
}

export function createTreeStore(): TreeStore {
  const state = writable<State>({ levels: new Map(), expanded: new Set() });
  const set = (fn: (s: State) => void) =>
    state.update((s) => {
      const n = { levels: new Map(s.levels), expanded: new Set(s.expanded) };
      fn(n);
      return n;
    });
  return {
    state: { subscribe: state.subscribe },
    rows: derived(state, (s) => buildRows(s.levels, s.expanded)),
    root: derived(state, (s) => s.levels.get('')),
    has: (p) => get(state).levels.get(p)?.status === 'ready',
    beginLoad: (p) =>
      set((s) => {
        const prev = s.levels.get(p);
        s.levels.set(p, { status: 'loading', level: prev?.level });
      }),
    resolve: (p, level) => set((s) => s.levels.set(p, { status: 'ready', level })),
    fail: (p, error) =>
      set((s) => {
        const prev = s.levels.get(p);
        s.levels.set(p, { status: 'error', error, level: prev?.level });
      }),
    toggle(p) {
      let open = false;
      set((s) => {
        if (s.expanded.has(p)) s.expanded.delete(p);
        else {
          s.expanded.add(p);
          open = true;
        }
      });
      return open;
    },
    expand: (ps) => set((s) => ps.forEach((p) => s.expanded.add(p))),
    isExpanded: (p) => get(state).expanded.has(p),
    invalidateAll() {
      const s = get(state);
      const keep = ['', ...[...s.expanded]];
      set((n) => {
        for (const k of [...n.levels.keys()]) if (!keep.includes(k)) n.levels.delete(k);
      });
      return keep;
    },
  };
}
