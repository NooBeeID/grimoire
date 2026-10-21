import { writable, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { KnowledgeFile } from '../../base';

export type ViewerState =
  | { status: 'idle' }
  | { status: 'loading'; path: string }
  | { status: 'ready'; file: KnowledgeFile }
  | { status: 'notFound'; path: string }
  | { status: 'error'; path: string; error: ApiError };

export interface ViewerStore extends Readable<ViewerState> {
  begin(path: string): void;
  resolve(file: KnowledgeFile): void;
  fail(path: string, e: ApiError): void;
}

export function createViewerStore(): ViewerStore {
  const s = writable<ViewerState>({ status: 'idle' });
  let latest: string | null = null;
  return {
    subscribe: s.subscribe,
    begin(path) {
      latest = path;
      s.set({ status: 'loading', path });
    },
    resolve(file) {
      if (file.path !== latest) return;
      s.set({ status: 'ready', file });
    },
    fail(path, e) {
      if (path !== latest) return;
      s.set(e.status === 404 ? { status: 'notFound', path } : { status: 'error', path, error: e });
    },
  };
}
