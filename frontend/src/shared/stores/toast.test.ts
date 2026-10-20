import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../services/http';
import { toast, toasts } from './toast';

beforeEach(() => {
  vi.useFakeTimers();
  toast.clear();
});
afterEach(() => vi.useRealTimers());

describe('toast', () => {
  it('auto-dismisses after 5000/8000 ms', () => {
    toast.push({ type: 'info', message: 'a' });
    toast.push({ type: 'error', message: 'b' });
    vi.advanceTimersByTime(5000);
    expect(get(toasts).map((x) => x.message)).toEqual(['b']);
    vi.advanceTimersByTime(3000);
    expect(get(toasts)).toHaveLength(0);
  });
  it('timeoutMs 0 persists', () => {
    toast.push({ type: 'info', message: 'a', timeoutMs: 0 });
    vi.advanceTimersByTime(60_000);
    expect(get(toasts)).toHaveLength(1);
  });
  it('dismiss clears timer', () => {
    const id = toast.push({ type: 'info', message: 'a' });
    toast.dismiss(id);
    expect(get(toasts)).toHaveLength(0);
    expect(vi.getTimerCount()).toBe(0);
  });
  it('keeps at most 5', () => {
    for (let i = 0; i < 7; i++) toast.push({ type: 'info', message: String(i) });
    expect(get(toasts).map((x) => x.message)).toEqual(['2', '3', '4', '5', '6']);
  });
  it('fromError', () => {
    toast.fromError(new ApiError({ status: 400, code: 'X', message: 'm', traceId: 'tr' }));
    expect(get(toasts)[0]).toMatchObject({ message: 'm', traceId: 'tr', type: 'error' });
    toast.clear();
    toast.fromError(new Error('boom'));
    expect(get(toasts)[0]?.message).toBe('Terjadi kesalahan pada server');
  });
});
