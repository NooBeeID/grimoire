import { get } from 'svelte/store';
import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { mockUsageAll, mockUsageSection } from '../mocks/usage';
import type { AgentUsage, UsageFilter, UsageSection } from '../types';
import { createUsageStore } from './usage.store';

const withProject: UsageFilter = { range: '7d', projectId: mockUsageAll.project_id };
const noProject: UsageFilter = { range: '7d', projectId: null };
const ok = async (_f: UsageFilter, s: UsageSection) => mockUsageSection(mockUsageAll, s);
const tick = () => new Promise((r) => setTimeout(r, 0));

describe('createUsageStore', () => {
  it('loads four sections in parallel with project', async () => {
    const fetcher = vi.fn(ok);
    const st = createUsageStore(fetcher);
    st.load(withProject);
    await tick();
    expect(fetcher.mock.calls.map((c) => c[1]).sort()).toEqual(['keys', 'top_files', 'unread_files', 'zero_queries']);
    expect(get(st).top_files.status).toBe('ready');
  });
  it('skips project sections without project', async () => {
    const fetcher = vi.fn(ok);
    const st = createUsageStore(fetcher);
    st.load(noProject);
    await tick();
    expect(fetcher.mock.calls.map((c) => c[1]).sort()).toEqual(['keys', 'zero_queries']);
    expect(get(st).top_files.status).toBe('needs_project');
    expect(get(st).unread_files.status).toBe('needs_project');
  });
  it('maps PROJECT_REQUIRED to needs_project', async () => {
    const st = createUsageStore(async (_f, s) => ({
      ...mockUsageSection(mockUsageAll, s),
      top_files: { available: false, error_code: 'PROJECT_REQUIRED', items: [] },
    }));
    st.load(withProject);
    await tick();
    expect(get(st).top_files.status).toBe('needs_project');
  });
  it('marks unavailable section as error only', async () => {
    const st = createUsageStore(async (f, s) =>
      s === 'zero_queries'
        ? ({ ...mockUsageSection(mockUsageAll, s), zero_queries: { available: false, error_code: 'UNAVAILABLE', items: [] } } as AgentUsage)
        : ok(f, s),
    );
    st.load(withProject);
    await tick();
    expect(get(st).zero_queries).toMatchObject({ status: 'error', error: 'unavailable' });
    expect(get(st).keys.status).toBe('ready');
  });
  it('http error affects one section', async () => {
    const err = new ApiError({ status: 500, code: 'INTERNAL', message: 'x' });
    const st = createUsageStore(async (f, s) => (s === 'keys' ? Promise.reject(err) : ok(f, s)));
    st.load(withProject);
    await tick();
    expect(get(st).keys).toMatchObject({ status: 'error', error: err });
    expect(get(st).top_files.status).toBe('ready');
  });
  it('empty items -> empty status', async () => {
    const st = createUsageStore(async (_f, s) =>
      mockUsageSection({ ...mockUsageAll, per_key: { available: true, items: [] } }, s),
    );
    st.load(withProject);
    await tick();
    expect(get(st).keys.status).toBe('empty');
  });
  it('retry reloads one section', async () => {
    const fetcher = vi.fn(ok);
    const st = createUsageStore(fetcher);
    st.load(withProject);
    await tick();
    fetcher.mockClear();
    await st.retry('keys');
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher.mock.calls[0]![1]).toBe('keys');
  });
  it('abortsStaleLoad', async () => {
    const resolvers: Record<string, (v: AgentUsage) => void> = {};
    const fetcher = vi.fn(
      (f: UsageFilter, s: UsageSection) => new Promise<AgentUsage>((r) => (resolvers[`${f.range}-${s}`] = r)),
    );
    const st = createUsageStore(fetcher);
    st.load({ range: '7d', projectId: null });
    st.load({ range: '30d', projectId: null });
    resolvers['30d-keys']!(mockUsageSection({ ...mockUsageAll, per_key: { available: true, items: [] } }, 'keys'));
    await tick();
    resolvers['7d-keys']!(mockUsageSection(mockUsageAll, 'keys'));
    await tick();
    expect(get(st).key).toBe('30d|');
    expect(get(st).keys.status).toBe('empty');
  });
  it('keeps truncated flag for unread_files', async () => {
    const st = createUsageStore(async (_f, s) =>
      mockUsageSection({ ...mockUsageAll, unread_files: { available: true, truncated: true, items: mockUsageAll.unread_files!.items } }, s),
    );
    st.load(withProject);
    await tick();
    expect(get(st).unread_files.truncated).toBe(true);
  });
});
