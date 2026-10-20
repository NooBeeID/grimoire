import { writable, type Readable } from 'svelte/store';

export type Role = 'super_admin' | 'developer';
export interface Identity {
  userId: string;
  email: string;
  name: string;
  role: Role;
  mustChangePassword: boolean;
}
export type SessionStatus = 'unknown' | 'anonymous' | 'authenticated';
export interface SessionState {
  status: SessionStatus;
  identity: Identity | null;
}

const initial: SessionState = { status: 'unknown', identity: null };
const store = writable<SessionState>(initial);

export const session: Readable<SessionState> = { subscribe: store.subscribe };

export function setIdentity(i: Identity | null): void {
  store.set(i ? { status: 'authenticated', identity: i } : { status: 'anonymous', identity: null });
}

export function resetSession(): void {
  store.set(initial);
}

