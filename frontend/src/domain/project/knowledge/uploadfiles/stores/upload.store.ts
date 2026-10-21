import { derived, writable, type Readable } from 'svelte/store';
import type { ReportItem, UploadPlan } from '../types';

export type Phase = 'pick' | 'review' | 'uploading' | 'done' | 'failed';
export type Filter = 'all' | 'failed' | 'updated' | 'created' | 'unchanged' | 'skipped';

interface State {
  phase: Phase;
  plan: UploadPlan | null;
  report: ReportItem[];
  targetErrors: Map<string, string>;
  filter: Filter;
}

export interface UploadStore extends Readable<State> {
  counts: Readable<Record<'created' | 'updated' | 'unchanged' | 'skipped' | 'rejected' | 'unsupported', number>>;
  filtered: Readable<ReportItem[]>;
  setPlan(plan: UploadPlan): void;
  start(): void;
  addReport(items: ReportItem[]): void;
  targetError(key: string, message: string): void;
  finish(ok: boolean): void;
  setFilter(f: Filter): void;
  reset(): void;
}

const init = (): State => ({ phase: 'pick', plan: null, report: [], targetErrors: new Map(), filter: 'all' });

export function createUploadStore(): UploadStore {
  const s = writable<State>(init());
  const up = (fn: (x: State) => State) => s.update(fn);
  return {
    subscribe: s.subscribe,
    counts: derived(s, (x) => {
      const c = { created: 0, updated: 0, unchanged: 0, skipped: 0, rejected: 0, unsupported: 0 };
      for (const r of x.report) c[r.status]++;
      return c;
    }),
    filtered: derived(s, (x) => {
      switch (x.filter) {
        case 'failed': return x.report.filter((r) => r.status === 'rejected' || r.status === 'unsupported');
        case 'all': return x.report;
        default: return x.report.filter((r) => r.status === x.filter);
      }
    }),
    setPlan: (plan) => up((x) => ({ ...x, plan, phase: 'review' })),
    start: () =>
      up((x) => ({ ...x, phase: 'uploading', report: [...(x.plan?.skipped ?? []), ...(x.plan?.unsupported ?? [])], targetErrors: new Map() })),
    addReport: (items) => up((x) => ({ ...x, report: [...x.report, ...items] })),
    targetError: (key, message) => up((x) => ({ ...x, targetErrors: new Map(x.targetErrors).set(key, message) })),
    finish: (ok) => up((x) => ({ ...x, phase: ok ? 'done' : 'failed' })),
    setFilter: (filter) => up((x) => ({ ...x, filter })),
    reset: () => s.set(init()),
  };
}
