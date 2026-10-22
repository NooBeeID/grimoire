import { get } from 'svelte/store';
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockSummary24hProject, mockSummary7d, mockSummaryQualityUnavailable } from '../mocks/summary';
import type { SummaryFilter, UsageSummary } from '../types';
import { createSummaryStore } from './summary.store';

const F7: SummaryFilter = { range: '7d', projectId: null };
const F24: SummaryFilter = { range: '24h', projectId: null };
const err = () => new ApiError({ status: 500, code: 'INTERNAL', message: 'x' });

function deferred() {
  let resolve!: (v: UsageSummary) => void;
  let reject!: (e: unknown) => void;
  const p = new Promise<UsageSummary>((a, b) => ((resolve = a), (reject = b)));
  return { p, resolve, reject };
}

describe('summary store', () => {
  it('aborts the old request and drops a late response', async () => {
    const d1 = deferred();
    const d2 = deferred();
    const signals: AbortSignal[] = [];
    const queue = [d1, d2];
    
    const s = createSummaryStore((_f, signal) => {
      signals.push(signal!);
      return queue.shift()!.p;
    });
    const a = s.load(F7);
    const b = s.load(F24);
    expect(signals[0]!.aborted).toBe(true);
    d2.resolve(mockSummary24hProject);
    await b;
    d1.resolve(mockSummary7d);
    await a;
    expect(get(s).data?.range).toBe('24h');
    expect(get(s).status).toBe('ready');
    expect(get(s).loadedAt).toEqual(new Date(mockSummary24hProject.to));
  });

  it('failure -> error with null data', async () => {
    const s = createSummaryStore(async () => {
      throw err();
    });
    await s.load(F7);
    expect(get(s)).toMatchObject({ status: 'error', data: null });
  });

  it('reload keeps old data and records refreshError', async () => {
    let fail = false;
    const s = createSummaryStore(async () => {
      if (fail) throw err();
      return mockSummary7d;
    });
    await s.load(F7);
    fail = true;
    const p = s.reload();
    expect(get(s).refreshing).toBe(true);
    await p;
    expect(get(s)).toMatchObject({ status: 'ready', refreshing: false });
    expect(get(s).refreshError).not.toBeNull();
    expect(get(s).data).toEqual(mockSummary7d);
  });

  it('retryQuality only flips qualityRetrying, then replaces the snapshot', async () => {
    let next: UsageSummary = mockSummaryQualityUnavailable;
    const s = createSummaryStore(async () => next);
    await s.load({ range: '24h', projectId: mockSummary24hProject.project_id });
    expect(get(s).data?.search_quality).toEqual({ available: false, error_code: 'UNAVAILABLE' });
    next = mockSummary24hProject;
    const p = s.retryQuality();
    expect(get(s)).toMatchObject({ qualityRetrying: true, status: 'ready' });
    await p;
    expect(get(s).qualityRetrying).toBe(false);
    expect(get(s).data?.search_quality?.available).toBe(true);
  });
});
