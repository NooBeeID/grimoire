import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockConfig, mockKnowledgeReport } from '../../base';
import { createRunSyncStore } from './runsync.store';

afterEach(() => vi.unstubAllGlobals());
const input = { kind: 'knowledge' as const, url: 'https://github.com/a/b', branch: 'main', folder: 'docs' };
const okBody = { report: mockKnowledgeReport(), config: mockConfig({ last_result: null }) };

describe('runsync store', () => {
  it('200 -> done', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(okBody), { status: 200 })));
    const s = createRunSyncStore();
    await s.start('p', input);
    expect(get(s).status).toBe('done');
  });
  it('start while running is ignored (one fetch)', async () => {
    let resolve!: (r: Response) => void;
    const f = vi.fn(() => new Promise<Response>((r) => (resolve = r)));
    vi.stubGlobal('fetch', f);
    const s = createRunSyncStore();
    const first = s.start('p', input);
    await s.start('p', input);
    expect(f).toHaveBeenCalledTimes(1);
    resolve(new Response(JSON.stringify(okBody), { status: 200 }));
    await first;
    expect(get(s).status).toBe('done');
  });
  it('422 -> error with SyncError', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'CREDENTIAL_MISSING', message: 'x', details: { host: 'github.com' } } }), { status: 422 })));
    const s = createRunSyncStore();
    await s.start('p', input);
    const st = get(s);
    expect(st.status === 'error' && st.error.code).toBe('CREDENTIAL_MISSING');
  });
  it('abort -> idle', async () => {
    vi.stubGlobal('fetch', vi.fn((_u: string, init?: RequestInit) => new Promise<Response>((_, rej) => init!.signal!.addEventListener('abort', () => rej(new DOMException('x', 'AbortError'))))));
    const s = createRunSyncStore();
    const p = s.start('p', input);
    s.abort();
    await p;
    expect(get(s).status).toBe('idle');
  });
  it('reset from done', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(okBody), { status: 200 })));
    const s = createRunSyncStore();
    await s.start('p', input);
    s.reset();
    expect(get(s).status).toBe('idle');
  });
});
