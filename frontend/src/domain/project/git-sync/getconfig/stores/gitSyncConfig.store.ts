import { writable, type Readable } from 'svelte/store';
import { ApiError, isApiError } from '../../../../../shared/services/http';
import type { GitSyncConfig, GitSyncReport, Kind } from '../../base';
import { getConfig } from '../services/getconfig.service';
import type { GetConfigResponse } from '../types';

export type ConfigState =
  | { status: 'idle' }
  | { status: 'loading'; previous: GetConfigResponse | null }
  | { status: 'ready'; data: GetConfigResponse }
  | { status: 'error'; error: ApiError; previous: GetConfigResponse | null };

export interface GitSyncConfigStore extends Readable<ConfigState> {
  load(projectId: string, kind: Kind): Promise<void>;
  setFromRun(config: GitSyncConfig, report: GitSyncReport): void;
  reset(): void;
}

// Satu instance per dialog.
export function createGitSyncConfigStore(): GitSyncConfigStore {
  const state = writable<ConfigState>({ status: 'idle' });
  let snap: ConfigState = { status: 'idle' };
  state.subscribe((s) => (snap = s));
  let ctrl: AbortController | null = null;
  let key: string | null = null;

  const prevOf = (s: ConfigState): GetConfigResponse | null =>
    s.status === 'ready' ? s.data : s.status === 'loading' || s.status === 'error' ? s.previous : null;

  return {
    subscribe: state.subscribe,
    async load(projectId, kind) {
      const k = `${projectId}:${kind}`;
      if (snap.status === 'loading' && key === k) return;
      const sameKey = key === k;
      ctrl?.abort();
      const c = new AbortController();
      ctrl = c;
      key = k;
      state.set({ status: 'loading', previous: sameKey ? prevOf(snap) : null });
      try {
        const data = await getConfig(projectId, kind, c.signal);
        if (!c.signal.aborted) state.set({ status: 'ready', data });
      } catch (e) {
        if (c.signal.aborted || (e instanceof DOMException && e.name === 'AbortError')) return;
        const error = isApiError(e) ? e : new ApiError({ status: 0, code: 'INTERNAL', message: String(e) });
        state.update((s) => ({ status: 'error', error, previous: prevOf(s) }));
      }
    },
    // Respons run membawa last_result = null; isi dari report agar ringkasan langsung benar tanpa fetch ulang.
    setFromRun(config, report) {
      state.update((s) => {
        const base = prevOf(s);
        if (!base) return s;
        return { status: 'ready', data: { ...base, has_credential: true, credential_unreadable: false, in_progress: false, config: { ...config, last_result: report } } };
      });
    },
    reset() {
      ctrl?.abort();
      ctrl = null;
      key = null;
      state.set({ status: 'idle' });
    },
  };
}
