import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { mockConfig, mockKnowledgeReport } from '../../base';
import { controlledStream, sseFrame } from '../streamTestUtil';
import { createRunSyncStore, type RunState } from './runsync.store';

afterEach(() => vi.unstubAllGlobals());
const input = { kind: 'knowledge' as const, url: 'https://github.com/a/b', branch: 'main', folder: 'docs' };
const okBody = { report: mockKnowledgeReport(), config: mockConfig({ last_result: null }) };
const tick = () => new Promise((r) => setTimeout(r, 0));
const progressOf = (s: RunState) => (s.status === 'running' ? s.progress : null);

function setup() {
  const c = controlledStream();
  const f = vi.fn(async () => c.res);
  vi.stubGlobal('fetch', f);
  let nowMs = 1_000_000;
  const store = createRunSyncStore({ now: () => nowMs });
  return { c, f, store };
}

describe('runsync store', () => {
  it('listing -> fetching -> waiting -> fetching -> saving -> done', async () => {
    const { c, store } = setup();
    const p = store.start('p', input);
    expect(progressOf(get(store))).toEqual({ phase: 'listing' });
    c.push(sseFrame('start', { total: 42 }));
    await tick();
    expect(progressOf(get(store))).toEqual({ phase: 'fetching', done: 0, total: 42, path: '' });
    c.push(sseFrame('progress', { done: 7, total: 42, path: 'docs/a.md' }));
    await tick();
    expect(progressOf(get(store))).toMatchObject({ phase: 'fetching', done: 7 });
    c.push(sseFrame('waiting', { retry_after_seconds: 38, reason: 'rate_limit_exhausted' }));
    await tick();
    expect(progressOf(get(store))).toEqual({ phase: 'waiting', done: 7, total: 42, path: 'docs/a.md', retryAfter: 38, until: 1_000_000 + 38_000 });
    c.push(sseFrame('progress', { done: 8, total: 42, path: 'docs/b.md' }));
    await tick();
    expect(progressOf(get(store))).toMatchObject({ phase: 'fetching', done: 8 });
    c.push(sseFrame('saving', {}));
    await tick();
    expect(progressOf(get(store))).toEqual({ phase: 'saving', total: 42 });
    c.push(sseFrame('done', okBody));
    const r = await p;
    expect(r.status).toBe('done');
  });

  it('waiting before start has total null, then start -> fetching', async () => {
    const { c, store } = setup();
    const p = store.start('p', input);
    c.push(sseFrame('waiting', { retry_after_seconds: 5, reason: 'x' }));
    await tick();
    expect(progressOf(get(store))).toMatchObject({ phase: 'waiting', total: null, done: 0 });
    c.push(sseFrame('start', { total: 3 }));
    await tick();
    expect(progressOf(get(store))).toMatchObject({ phase: 'fetching', total: 3 });
    c.close();
    await p;
  });

  it('ignores progress going backwards', async () => {
    const { c, store } = setup();
    const p = store.start('p', input);
    c.push(sseFrame('progress', { done: 7, total: 9, path: 'a' }) + sseFrame('progress', { done: 5, total: 9, path: 'b' }));
    await tick();
    expect(progressOf(get(store))).toMatchObject({ done: 7, path: 'a' });
    c.close();
    await p;
  });

  it('error event RATE_LIMITED -> error{lost:false, retryAfter}; NETWORK exception -> lost', async () => {
    const a = setup();
    const p = a.store.start('p', input);
    a.c.push(sseFrame('error', { code: 'RATE_LIMITED', message: 'x', details: { retry_after_seconds: 1712 } }));
    const r = await p;
    expect(r.status === 'error' && r.lost).toBe(false);
    expect(r.status === 'error' && r.error.retryAfter).toBe(1712);

    const b = setup();
    const q = b.store.start('p', input);
    b.c.push(sseFrame('start', { total: 2 }));
    b.c.close();
    const r2 = await q;
    expect(r2.status === 'error' && r2.lost).toBe(true);
  });

  it('events after done are ignored; double start makes one request; abort -> idle', async () => {
    const { c, f, store } = setup();
    const p = store.start('p', input);
    await store.start('p', input);
    expect(f).toHaveBeenCalledTimes(1);
    c.push(sseFrame('done', okBody) + sseFrame('progress', { done: 1, total: 1, path: 'late' }));
    await p;
    await tick();
    expect(get(store).status).toBe('done');

    const x = setup();
    const q = x.store.start('p', input);
    x.store.abort();
    await q;
    expect(get(x.store).status).toBe('idle');
  });

  it('422 before the stream -> error, not lost', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: { code: 'CREDENTIAL_MISSING', message: 'x', details: { host: 'github.com' } } }), { status: 422 })));
    const s = createRunSyncStore();
    await s.start('p', input);
    const st = get(s);
    expect(st.status === 'error' && st.error.code).toBe('CREDENTIAL_MISSING');
    expect(st.status === 'error' && st.lost).toBe(false);
  });

  it('reset from done', async () => {
    const { c, store } = setup();
    const p = store.start('p', input);
    c.push(sseFrame('done', okBody));
    await p;
    store.reset();
    expect(get(store).status).toBe('idle');
  });
});
