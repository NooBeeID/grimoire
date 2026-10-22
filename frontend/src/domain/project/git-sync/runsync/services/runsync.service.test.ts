import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockConfig, mockKnowledgeReport } from '../../base';
import { CLIENT_TIMEOUT_MS, runSync } from './runsync.service';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
const okBody = { report: mockKnowledgeReport(), config: mockConfig({ last_result: null }) };
function stub(status = 200, body: unknown = okBody) {
  const f = vi.fn(async (_u: string, _i?: RequestInit) => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal('fetch', f);
  return f;
}
const rejectOnAbort = () =>
  vi.fn((_u: string, init?: RequestInit) => new Promise<Response>((_, rej) => init!.signal!.addEventListener('abort', () => rej((init!.signal as AbortSignal).reason))));

describe('runSync', () => {
  it('knowledge body trims and carries target_prefix; CSRF header set', async () => {
    const f = stub();
    await runSync('p1', { kind: 'knowledge', url: ' https://github.com/a/b ', branch: ' main ', folder: ' docs ', target_prefix: ' guides ' });
    const init = f.mock.calls[0]![1]!;
    expect(f.mock.calls[0]![0]).toBe('/api/projects/p1/git-sync/run');
    expect(JSON.parse(init.body as string)).toEqual({ kind: 'knowledge', url: 'https://github.com/a/b', branch: 'main', folder: 'docs', target_prefix: 'guides' });
    expect((init.headers as Record<string, string>)['X-Requested-With']).toBe('grimoire');
  });
  it('api_spec body has no target_prefix', async () => {
    const f = stub();
    await runSync('p1', { kind: 'api_spec', url: 'https://github.com/a/b', branch: 'main', folder: 'api', target_prefix: 'x' });
    expect(JSON.parse(f.mock.calls[0]![1]!.body as string)).not.toHaveProperty('target_prefix');
  });
  it('invalid body -> INVALID_RESPONSE', async () => {
    stub(200, { report: {} });
    await expect(runSync('p', { kind: 'knowledge', url: 'u', branch: '', folder: '' })).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });
  it('client timeout at 130 s surfaces a TimeoutError', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', rejectOnAbort());
    const p = runSync('p', { kind: 'knowledge', url: 'u', branch: '', folder: '' });
    const assertion = expect(p).rejects.toMatchObject({ name: 'TimeoutError' });
    await vi.advanceTimersByTimeAsync(CLIENT_TIMEOUT_MS + 1);
    await assertion;
  });
  it('user signal aborts the request', async () => {
    vi.stubGlobal('fetch', rejectOnAbort());
    const ctrl = new AbortController();
    const p = runSync('p', { kind: 'knowledge', url: 'u', branch: '', folder: '' }, ctrl.signal);
    ctrl.abort();
    await expect(p).rejects.toMatchObject({ name: 'AbortError' });
  });
});
