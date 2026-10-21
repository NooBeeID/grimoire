import { writable } from 'svelte/store';
import type { ApiError } from '../../../../../shared/services/http';
import type { Candidate } from '../types';

export interface CandidateState {
  q: string;
  status: 'idle' | 'loading' | 'ready' | 'error';
  items: Candidate[];
  error: ApiError | null;
  activeIndex: number; // -1 = tidak ada
}

const initial = (): CandidateState => ({ q: '', status: 'idle', items: [], error: null, activeIndex: -1 });

export function createCandidateSearchStore() {
  const state = writable<CandidateState>(initial());
  let current = initial();
  let seq = 0;
  const set = (s: CandidateState) => {
    current = s;
    state.set(s);
  };
  return {
    subscribe: state.subscribe,
    // Query diubah: q kosong -> idle; selain itu loading sampai hasil terakhir tiba.
    setQuery(q: string) {
      set({ q, status: q.trim() === '' ? 'idle' : 'loading', items: [], error: null, activeIndex: -1 });
    },
    begin(n: number) {
      seq = n;
      set({ ...current, status: 'loading', error: null });
    },
    resolve(n: number, items: Candidate[]) {
      if (n !== seq) return;
      set({ ...current, status: 'ready', items, error: null, activeIndex: -1 });
    },
    fail(n: number, error: ApiError) {
      if (n !== seq) return;
      set({ ...current, status: 'error', items: [], error, activeIndex: -1 });
    },
    move(delta: number) {
      const n = current.items.length;
      if (n === 0) return;
      let i = current.activeIndex;
      if (i < 0) i = delta > 0 ? 0 : n - 1;
      else i = (i + delta + n) % n;
      set({ ...current, activeIndex: i });
    },
    // Membatalkan hasil yang masih berjalan dengan menaikkan seq.
    clear() {
      seq += 1;
      set(initial());
    },
  };
}
export type CandidateSearchStore = ReturnType<typeof createCandidateSearchStore>;
