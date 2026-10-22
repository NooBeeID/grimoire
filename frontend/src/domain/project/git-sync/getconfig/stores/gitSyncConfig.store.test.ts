import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockConfig, mockKnowledgeReport } from '../../base';
import { createGitSyncConfigStore } from './gitSyncConfig.store';

afterEach(() => vi.unstubAllGlobals());
const resp = (config: unknown) => ({ kind: 'knowledge', feature_enabled: true, has_credential: true, credential_unreadable: false, config });
const ok = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

describe('gitSyncConfig store', () => {
  it('a second load for the same key while loading issues one fetch', async () => {
    const f = vi.fn(async () => ok(resp(null)));
    vi.stubGlobal('fetch', f);
    const s = createGitSyncConfigStore();
    await Promise.all([s.load('p', 'knowledge'), s.load('p', 'knowledge')]);
    expect(f).toHaveBeenCalledTimes(1);
    expect(get(s).status).toBe('ready');
  });
  it('a new load aborts the previous one', async () => {
    const signals: AbortSignal[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_u: string, init?: RequestInit) => {
        signals.push(init!.signal as AbortSignal);
        if (signals.length === 1) return new Promise<Response>((_, rej) => signals[0]!.addEventListener('abort', () => rej(new DOMException('x', 'AbortError'))));
        return ok(resp(null));
      }),
    );
    const s = createGitSyncConfigStore();
    const first = s.load('p', 'knowledge');
    await s.load('p', 'api_spec');
    await first;
    expect(signals[0]!.aborted).toBe(true);
    expect(get(s).status).toBe('ready');
  });
  it('setFromRun fills last_result from the report', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ok(resp(null))));
    const s = createGitSyncConfigStore();
    await s.load('p', 'knowledge');
    const report = mockKnowledgeReport();
    s.setFromRun(mockConfig({ last_result: null }), report);
    const st = get(s);
    expect(st.status === 'ready' && st.data.config?.last_result).toEqual(report);
    expect(st.status === 'ready' && st.data.has_credential).toBe(true);
  });
  it('setFromRun clears in_progress that load reported', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ok({ ...resp(mockConfig()), in_progress: true })));
    const s = createGitSyncConfigStore();
    await s.load('p', 'knowledge');
    expect((get(s) as { data: { in_progress: boolean } }).data.in_progress).toBe(true);
    s.setFromRun(mockConfig({ last_result: null }), mockKnowledgeReport());
    expect((get(s) as { data: { in_progress: boolean } }).data.in_progress).toBe(false);
  });
  it('error keeps previous data', async () => {
    const f = vi
      .fn()
      .mockResolvedValueOnce(ok(resp(mockConfig())))
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: 'INTERNAL', message: 'x', details: null } }), { status: 500 }));
    vi.stubGlobal('fetch', f);
    const s = createGitSyncConfigStore();
    await s.load('p', 'knowledge');
    await s.load('p', 'knowledge');
    const st = get(s);
    expect(st.status === 'error' && st.previous?.config?.branch).toBe('main');
  });
  it('reset -> idle', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ok(resp(null))));
    const s = createGitSyncConfigStore();
    await s.load('p', 'knowledge');
    s.reset();
    expect(get(s).status).toBe('idle');
  });
});
