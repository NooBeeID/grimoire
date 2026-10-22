import { writable, type Readable } from 'svelte/store';
import { runSyncStream } from '../services/runsync.service';
import { isStreamLost, toSyncError, type RunSyncInput, type RunSyncResponse, type SyncError, type SyncEvent } from '../types';

export type Progress =
  | { phase: 'listing' }
  | { phase: 'fetching'; done: number; total: number; path: string }
  | { phase: 'waiting'; done: number; total: number | null; path: string; retryAfter: number; until: number }
  | { phase: 'saving'; total: number };

export type RunState =
  | { status: 'idle' }
  | { status: 'running'; progress: Progress }
  | { status: 'done'; report: RunSyncResponse['report']; config: RunSyncResponse['config'] }
  | { status: 'error'; error: SyncError; lost: boolean };

export interface RunSyncStore extends Readable<RunState> {
  start(projectId: string, input: RunSyncInput): Promise<RunState>;
  abort(): void;
  reset(): void;
}

// Satu instance per dialog.
export function createRunSyncStore(deps: { now?: () => number } = {}): RunSyncStore {
  const now = deps.now ?? Date.now;
  const state = writable<RunState>({ status: 'idle' });
  let snap: RunState = { status: 'idle' };
  state.subscribe((s) => (snap = s));
  let ctrl: AbortController | null = null;

  function apply(c: AbortController, ev: SyncEvent) {
    if (c !== ctrl || snap.status !== 'running') return; // event setelah terminal/abort diabaikan
    const cur = snap.progress;
    const set = (progress: Progress) => state.set({ status: 'running', progress });
    switch (ev.type) {
      case 'start':
        set({ phase: 'fetching', done: 0, total: ev.total, path: '' });
        break;
      case 'progress':
        if (cur.phase === 'fetching' && ev.done < cur.done) return; // jaga monoton
        if (cur.phase === 'waiting' && ev.done < cur.done) return;
        set({ phase: 'fetching', done: ev.done, total: ev.total, path: ev.path });
        break;
      case 'waiting': {
        const base = cur.phase === 'fetching' || cur.phase === 'waiting' ? cur : null;
        set({
          phase: 'waiting',
          done: base?.done ?? 0,
          total: base?.total ?? null,
          path: base?.path ?? '',
          retryAfter: ev.retryAfter,
          until: now() + ev.retryAfter * 1000,
        });
        break;
      }
      case 'saving': {
        const total = cur.phase === 'fetching' ? cur.total : cur.phase === 'waiting' ? (cur.total ?? 0) : cur.phase === 'saving' ? cur.total : 0;
        set({ phase: 'saving', total });
        break;
      }
      case 'done':
        state.set({ status: 'done', report: ev.result.report, config: ev.result.config });
        break;
      case 'error':
        state.set({ status: 'error', error: ev.error, lost: false });
        break;
    }
  }

  return {
    subscribe: state.subscribe,
    async start(projectId, input) {
      if (snap.status === 'running') return snap;
      const c = new AbortController();
      ctrl = c;
      state.set({ status: 'running', progress: { phase: 'listing' } });
      try {
        await runSyncStream(projectId, input, (ev) => apply(c, ev), c.signal);
      } catch (e) {
        if (c.signal.aborted || c !== ctrl) return snap;
        if ((snap as RunState).status === 'running') {
          const err = toSyncError(e);
          state.set({ status: 'error', error: err, lost: isStreamLost(err) });
        }
      }
      return snap;
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
