import { get, writable, type Readable } from 'svelte/store';
import { isApiError } from '../services/http';
import { t } from './i18n';

export type ToastType = 'success' | 'error' | 'info';
export interface ToastItem {
  id: number;
  type: ToastType;
  message: string;
  traceId?: string;
  timeoutMs: number;
}

const MAX_TOASTS = 5;
const store = writable<ToastItem[]>([]);
const timers = new Map<number, ReturnType<typeof setTimeout>>();
let seq = 0;

export const toasts: Readable<ToastItem[]> = { subscribe: store.subscribe };

function clearTimer(id: number) {
  const h = timers.get(id);
  if (h !== undefined) clearTimeout(h);
  timers.delete(id);
}

function dismiss(id: number): void {
  clearTimer(id);
  store.update((l) => l.filter((x) => x.id !== id));
}

function push(i: { type: ToastType; message: string; traceId?: string; timeoutMs?: number }): number {
  const id = ++seq;
  const timeoutMs = i.timeoutMs ?? (i.type === 'error' ? 8000 : 5000);
  const item: ToastItem = { id, type: i.type, message: i.message, traceId: i.traceId, timeoutMs };
  store.update((l) => {
    const next = [...l, item];
    while (next.length > MAX_TOASTS) {
      const old = next.shift();
      if (old) clearTimer(old.id);
    }
    return next;
  });
  if (timeoutMs > 0) timers.set(id, setTimeout(() => dismiss(id), timeoutMs));
  return id;
}

function fromError(e: unknown): number {
  if (isApiError(e)) return push({ type: 'error', message: e.message, traceId: e.traceId });
  return push({ type: 'error', message: get(t)('error.internal') });
}

function clear(): void {
  for (const id of [...timers.keys()]) clearTimer(id);
  store.set([]);
}

export const toast = { push, dismiss, fromError, clear };
