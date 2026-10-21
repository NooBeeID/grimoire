import { z } from 'zod';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setLocale } from '../stores/i18n';
import { ApiError, setUnauthorizedHandler } from './http';
import { FakeXhr } from './fakeXhr';
import { postMultipart, type UploadProgress } from './upload';

const schema = z.object({ ok: z.boolean() });
const form = () => {
  const f = new FormData();
  f.append('files', new File(['x'], 'a.md'), 'docs/a.md');
  return f;
};

beforeEach(() => {
  setLocale('id');
  FakeXhr.last = null;
  vi.stubGlobal('XMLHttpRequest', FakeXhr);
});
afterEach(() => {
  vi.unstubAllGlobals();
  setUnauthorizedHandler(null);
});

describe('postMultipart', () => {
  it('sets CSRF and Accept headers but not Content-Type, with credentials', async () => {
    const p = postMultipart('/api/x', form(), { schema });
    const x = FakeXhr.last!;
    expect(x.method).toBe('POST');
    expect(x.withCredentials).toBe(true);
    expect(x.headers['X-Requested-With']).toBe('grimoire');
    expect(x.headers['Accept']).toBe('application/json');
    expect(x.headers['Content-Type']).toBeUndefined();
    x.respond(200, '{"ok":true}');
    await expect(p).resolves.toEqual({ ok: true });
  });

  it('rejects non same-origin urls', async () => {
    await expect(postMultipart('https://evil.test/x', form(), { schema })).rejects.toThrow();
    await expect(postMultipart('//evil.test/x', form(), { schema })).rejects.toThrow();
  });

  it('reports sending then processing progress', async () => {
    const seen: UploadProgress[] = [];
    const p = postMultipart('/api/x', form(), { schema, onProgress: (e) => seen.push(e) });
    const x = FakeXhr.last!;
    x.upload.onprogress?.({ lengthComputable: true, loaded: 5, total: 10 });
    x.upload.onload?.();
    x.respond(200, '{"ok":true}');
    await p;
    expect(seen).toEqual([
      { loaded: 5, total: 10, phase: 'sending' },
      { loaded: 10, total: 10, phase: 'processing' },
    ]);
  });

  it('maps schema mismatch and invalid json to INVALID_RESPONSE', async () => {
    const p1 = postMultipart('/api/x', form(), { schema });
    FakeXhr.last!.respond(200, '{"ok":1}');
    await expect(p1).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
    const p2 = postMultipart('/api/x', form(), { schema });
    FakeXhr.last!.respond(200, 'nope');
    await expect(p2).rejects.toMatchObject({ code: 'INVALID_RESPONSE' });
  });

  it('maps HTTP error envelope to ApiError with details', async () => {
    const p = postMultipart('/api/x', form(), { schema });
    FakeXhr.last!.respond(413, JSON.stringify({ error: { code: 'PAYLOAD_TOO_LARGE', message: 'besar', details: { reason: 'upload_too_large', max_bytes: 5 } } }));
    const e = await p.catch((x) => x);
    expect(e).toBeInstanceOf(ApiError);
    expect(e).toMatchObject({ status: 413, code: 'PAYLOAD_TOO_LARGE', details: { reason: 'upload_too_large' } });
  });

  it('notifies unauthorized on 401', async () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    window.history.pushState({}, '', '/projects/p1/knowledge');
    const p = postMultipart('/api/x', form(), { schema });
    FakeXhr.last!.respond(401, '');
    await expect(p).rejects.toMatchObject({ status: 401, code: 'INTERNAL' });
    expect(h).toHaveBeenCalledWith('/projects/p1/knowledge');
  });

  it('maps network error to NETWORK', async () => {
    const p = postMultipart('/api/x', form(), { schema });
    FakeXhr.last!.onerror?.();
    await expect(p).rejects.toMatchObject({ status: 0, code: 'NETWORK' });
  });

  it('aborts via signal and rejects AbortError', async () => {
    const c = new AbortController();
    const p = postMultipart('/api/x', form(), { schema, signal: c.signal });
    c.abort();
    expect(FakeXhr.last!.aborted).toBe(true);
    await expect(p).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('rejects immediately if signal already aborted', async () => {
    const c = new AbortController();
    c.abort();
    await expect(postMultipart('/api/x', form(), { schema, signal: c.signal })).rejects.toMatchObject({ name: 'AbortError' });
  });
});
