import { derived, writable, type Readable } from 'svelte/store';
import type { ConflictCurrent, KnowledgeFile } from '../../base';

interface State {
  base: { content: string; version: string };
  draft: string;
  saving: boolean;
  conflict: ConflictCurrent | null;
  compareOpen: boolean;
  error: string | null;
}

export interface EditorStore extends Readable<State> {
  dirty: Readable<boolean>;
  edit(text: string): void;
  saving(v: boolean): void;
  saved(file: KnowledgeFile): void;
  setConflict(c: ConflictCurrent): void;
  openCompare(open: boolean): void;
  acceptCurrent(): void;
  rebaseOnCurrent(): void;
  closeConflict(): void;
  setError(msg: string | null): void;
  restore(content: string, baseVersion: string): void;
}

export function createEditorStore(file: KnowledgeFile): EditorStore {
  const s = writable<State>({
    base: { content: file.content, version: file.version },
    draft: file.content,
    saving: false,
    conflict: null,
    compareOpen: false,
    error: null,
  });
  const up = (fn: (x: State) => State) => s.update(fn);
  return {
    subscribe: s.subscribe,
    dirty: derived(s, (x) => x.draft !== x.base.content),
    edit: (text) => up((x) => ({ ...x, draft: text })),
    saving: (v) => up((x) => ({ ...x, saving: v })),
    // Draf tetap bila pengguna mengetik selama simpan dan isinya berbeda.
    saved: (f) =>
      up((x) => ({ ...x, base: { content: f.content, version: f.version }, saving: false, conflict: null, compareOpen: false, error: null })),
    setConflict: (c) => up((x) => ({ ...x, conflict: c, saving: false })),
    openCompare: (open) => up((x) => ({ ...x, compareOpen: open })),
    acceptCurrent: () =>
      up((x) => (x.conflict ? { ...x, base: { content: x.conflict.content, version: x.conflict.version }, draft: x.conflict.content, conflict: null, compareOpen: false } : x)),
    // Timpa sadar: pakai versi terbaru sebagai dasar, draf tetap.
    rebaseOnCurrent: () =>
      up((x) => (x.conflict ? { ...x, base: { content: x.conflict.content, version: x.conflict.version }, conflict: null, compareOpen: false } : x)),
    closeConflict: () => up((x) => ({ ...x, conflict: null, compareOpen: false })),
    setError: (msg) => up((x) => ({ ...x, error: msg, saving: false })),
    restore: (content, baseVersion) => up((x) => ({ ...x, draft: content, base: { ...x.base, version: baseVersion } })),
  };
}
