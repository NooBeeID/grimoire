import { writable, type Readable } from 'svelte/store';
import { runSync } from '../services/runsync.service';
import { toSyncError, type RunSyncInput, type RunSyncResponse, type SyncError } from '../types';

export type RunState =
  | { status: 'idle' }
  | { status: 'running' }
  | { status: 'done'; report: RunSyncResponse['report']; config: RunSyncResponse['config'] }
  | { status: 'error'; error: SyncError };

export interface RunSyncStore extends Readable<RunState> {
  start(projectId: string, input: RunSyncInput): Promise<RunState>;
  abort(): void;
  reset(): void;
}

// Satu instance per dialog.
export function createRunSyncStore(): RunSyncStore {
  const state = writable<RunState>({ status: 'idle' });
  let snap: RunState = { status: 'idle' };
  state.subscribe((s) => (snap = s));
  let ctrl: AbortController | null = null;

  return {
    subscribe: state.subscribe,
    async start(projectId, input) {
      if (snap.status === 'running') return snap;
      const c = new AbortController();
      ctrl = c;
      state.set({ status: 'running' });
      try {
        const r = await runSync(projectId, input, c.signal);
        if (c.signal.aborted) return snap;
        const next: RunState = { status: 'done', report: r.report, config: r.config };
        state.set(next);
        return next;
      } catch (e) {
        if (c.signal.aborted) return snap;
        const err = toSyncError(e);
        const next: RunState = { status: 'error', error: err };
        state.set(next);
        return next;
      }
    },
    abort() {
      if (snap.status !== 'running') return;
      ctrl?.abort();
      state.set({ status: 'idle' });
    },
    reset() {
      ctrl?.abort();
      ctrl = null;
      state.set({ status: 'idle' });
    },
  };
}
