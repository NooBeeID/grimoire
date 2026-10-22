import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockConfig, mockKnowledgeReport } from '../../base';
import { controlledStream, sseFrame, sseResponse } from '../streamTestUtil';
import type { SyncEvent } from '../types';
import { IDLE_TIMEOUT_MS, runSyncStream } from './runsync.service';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
const okBody = { report: mockKnowledgeReport(), config: mockConfig({ last_result: null }) };
import type { RunSyncInput } from '../types';
const input: RunSyncInput = { kind: 'knowledge', url: 'u', branch: '', folder: '' };
const doneFrame = sseFrame('done', okBody);
function stubRes(res: Response | (() => Response)) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => (typeof res === 'function' ? res() : res));
  vi.stubGlobal('fetch', f);
  return f;
}
const rejectOnAbort = () =>
  vi.fn((_u: string, init?: RequestInit) => new Promise<Response>((_, rej) => init!.signal!.addEventListener('abort', () => rej((init!.signal as AbortSignal).reason))));
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const run = async (i = input, signal?: AbortSignal) => {
  const ev: SyncEvent[] = [];
  await runSyncStream('p1', i, (e) => ev.push(e), signal);
  return ev;
};

describe('runSyncStream', () => {
  it('forwards events in order and cancels the reader after done', async () => {
    const s = sseResponse([
      sseFrame('start', { total: 3 }),
      sseFrame('progress', { done: 1, total: 3, path: 'a.md' }),
      sseFrame('progress', { done: 2, total: 3, path: 'b.md' }),
      sseFrame('progress', { done: 3, total: 3, path: 'c.md' }),
      sseFrame('saving', {}),
      doneFrame,
      sseFrame('progress', { done: 9, total: 3, path: 'late.md' }),
    ], { keepOpen: true });
    stubRes(s.res);
    const ev = await run();
    expect(ev.map((e) => e.type)).toEqual(['start', 'progress', 'progress', 'progress', 'saving', 'done']);
    await vi.waitFor(() => expect(s.cancelled()).toBe(true));
  });

  it('sends SSE request with trimmed body; target_prefix only for knowledge', async () => {
    const f = stubRes(() => sseResponse([doneFrame]).res);
    await run({ kind: 'knowledge', url: ' https://github.com/a/b ', branch: ' main ', folder: ' docs ', target_prefix: ' guides ' });
    const init = f.mock.calls[0]![1]!;
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/git-sync/run');
    expect((init.headers as Record<string, string>).Accept).toBe('text/event-stream');
    expect(JSON.parse(init.body as string)).toEqual({ kind: 'knowledge', url: 'https://github.com/a/b', branch: 'main', folder: 'docs', target_prefix: 'guides' });
    f.mockImplementation(async () => sseResponse([doneFrame]).res);
    await run({ kind: 'api_spec', url: 'u', branch: '', folder: '', target_prefix: 'x' });
    expect(JSON.parse(f.mock.calls[1]![1]!.body as string)).not.toHaveProperty('target_prefix');
  });

  it('idle timeout: no bytes for 45 s -> TimeoutError; heartbeats keep it alive', async () => {
    vi.useFakeTimers();
    const c = controlledStream();
    stubRes(c.res);
    const p = run();
    const assertion = expect(p).rejects.toMatchObject({ name: 'TimeoutError' });
    for (let i = 0; i < 4; i++) {
      await vi.advanceTimersByTimeAsync(15_000);
      c.push(': ping\n\n');
    }
    await vi.advanceTimersByTimeAsync(IDLE_TIMEOUT_MS + 1);
    await assertion;
  });

  it.each([
    [409, 'SYNC_IN_PROGRESS'],
    [422, 'CREDENTIAL_MISSING'],
    [401, 'UNAUTHENTICATED'],
  ])('non-2xx %i -> ApiError %s before any event', async (status, code) => {
    stubRes(json(status, { error: { code, message: 'x', details: null } }));
    const ev: SyncEvent[] = [];
    await expect(runSyncStream('p', input, (e) => ev.push(e))).rejects.toMatchObject({ code, status });
    expect(ev).toEqual([]);
  });

  it('application/json response is treated as a done event', async () => {
    stubRes(json(200, okBody));
    const ev = await run();
    expect(ev).toHaveLength(1);
    expect(ev[0]!.type).toBe('done');
  });

  it('stream ending without a terminal event -> NETWORK', async () => {
    stubRes(sseResponse([sseFrame('start', { total: 1 })]).res);
    await expect(run()).rejects.toMatchObject({ code: 'NETWORK', status: 0 });
  });

  it('user abort -> AbortError, not TIMEOUT', async () => {
    vi.stubGlobal('fetch', rejectOnAbort());
    const ctrl = new AbortController();
    const p = run(input, ctrl.signal);
    ctrl.abort();
    await expect(p).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('malformed and unknown events are dropped', async () => {
    stubRes(sseResponse(['event: progress\ndata: {oops\n\n', sseFrame('mystery', {}), sseFrame('progress', { done: '1', total: 2, path: 'x' }), doneFrame]).res);
    expect((await run()).map((e) => e.type)).toEqual(['done']);
  });
});
