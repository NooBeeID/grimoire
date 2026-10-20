import { writable, type Readable } from 'svelte/store';
import { loadSession } from '../domain/core/auth/getme';
import { performLogout } from '../domain/core/auth/logout';
import { resetUsersState } from '../domain/core/auth/users';
import { setUnauthorizedHandler } from '../shared/services/http';
import { setIdentity } from '../shared/stores/session';
import { toast } from '../shared/stores/toast';
import { navigate } from './routes/router';

export type BootState = { kind: 'loading' } | { kind: 'ready' } | { kind: 'failed'; error: unknown };

const state = writable<BootState>({ kind: 'loading' });
export const bootState: Readable<BootState> = { subscribe: state.subscribe };

export async function boot(deps: { loadSession: () => Promise<void> } = { loadSession }): Promise<void> {
  state.set({ kind: 'loading' });
  try {
    await deps.loadSession();
    state.set({ kind: 'ready' });
  } catch (error) {
    state.set({ kind: 'failed', error });
  }
}

export function installAuthHandlers(): void {
  setUnauthorizedHandler((next) => {
    resetUsersState();
    setIdentity(null);
    navigate('/login?next=' + encodeURIComponent(next), { replace: true });
  });
}

export async function logoutAndLeave(deps: { performLogout: typeof performLogout } = { performLogout }): Promise<void> {
  const res = await deps.performLogout();
  if (!res.ok) {
    toast.fromError(res.error);
    return;
  }
  resetUsersState();
  navigate('/login', { replace: true });
  setIdentity(null);
}
