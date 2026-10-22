import { writable, get, type Readable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { GitCredentialList } from '../types';

export type GitCredentialsState =
  | { status: 'idle' }
  | { status: 'loading'; userId: string; previous: GitCredentialList | null }
  | { status: 'ready'; userId: string; data: GitCredentialList }
  | { status: 'error'; userId: string; error: ApiError; previous: GitCredentialList | null };

export interface GitCredentialsStore extends Readable<GitCredentialsState> {
  version: Readable<number>;
  begin(userId: string): void;
  resolve(userId: string, data: GitCredentialList): void;
  fail(userId: string, e: ApiError): void;
  invalidate(): void;
  reset(): void;
}

// Store hanya memegang metadata (last4, host). Token tidak pernah masuk ke sini.
export function createGitCredentialsStore(): GitCredentialsStore {
  const state = writable<GitCredentialsState>({ status: 'idle' });
  const version = writable(0);
  let current: string | null = null;

  const prevOf = (s: GitCredentialsState, userId: string): GitCredentialList | null => {
    if (s.status === 'idle' || s.userId !== userId) return null;
    return s.status === 'ready' ? s.data : s.previous;
  };

  return {
    subscribe: state.subscribe,
    version: { subscribe: version.subscribe },
    begin(userId) {
      current = userId;
      state.update((s) => ({ status: 'loading', userId, previous: prevOf(s, userId) }));
    },
    resolve(userId, data) {
      if (userId !== current) return;
      state.set({ status: 'ready', userId, data });
    },
    fail(userId, error) {
      if (userId !== current) return;
      state.update((s) => ({ status: 'error', userId, error, previous: prevOf(s, userId) }));
    },
    invalidate() {
      version.update((n) => n + 1);
    },
    reset() {
      current = null;
      state.set({ status: 'idle' });
    },
  };
}

export const gitCredentials = createGitCredentialsStore();
export const snapshot = (s: GitCredentialsStore): GitCredentialsState => get(s);
