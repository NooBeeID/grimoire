import { writable, type Readable } from 'svelte/store';

// null = belum diketahui (respons login tidak membawa email_enabled).
const store = writable<boolean | null>(null);
export const mailStatus: Readable<boolean | null> = { subscribe: store.subscribe };
export function setMailStatus(v: boolean | null): void {
  store.set(v);
}
export function resetMailStatus(): void {
  store.set(null);
}
