/* eslint-disable @typescript-eslint/no-explicit-any */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { z } from 'zod';
import { ApiError, del, get, getBlob, isApiError, notifyUnauthorized, openStream, post, request, setUnauthorizedHandler, toApiError } from './http';
import { HealthStatus } from '../validation/error';
import { setLocale } from '../stores/i18n';

const fetchMock = vi.fn();

function json(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json', ...headers } });
}
function envelope(code: string, status: number, extra: Record<string, unknown> = {}) {
  return json({ error: { code, message: 'pesan server', details: null, ...extra }, trace_id: 't1' }, status);
}
function lastInit(): RequestInit & { headers: Record<string, string> } {
  return fetchMock.mock.calls.at(-1)![1];
}

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock);
  fetchMock.mockReset();
  setLocale('id');
  setUnauthorizedHandler(null);
  history.pushState({}, '', '/');
});
afterEach(() => vi.unstubAllGlobals());

describe('request headers and body', () => {
  it.each([
    ['GET', false],
    ['HEAD', false],
    ['POST', true],
    ['PUT', true],
    ['PATCH', true],
    ['DELETE', true],
    ['post', true],
  ])('CSRF header for %s = %s', async (method, expected) => {
    fetchMock.mockImplementation(async () => json({}));
    await request(method, '/api/x');
    const h = lastInit().headers;
    if (expected) expect(h['X-Requested-With']).toBe('grimoire');
    else expect(h['X-Requested-With']).toBeUndefined();
  });

  it('uses same-origin credentials and Accept', async () => {
    fetchMock.mockImplementation(async () => json({}));
    await get('/api/x');
    expect(lastInit().credentials).toBe('same-origin');
    expect(lastInit().headers.Accept).toBe('application/json');
  });

  it('serializes JSON, FormData and undefined bodies', async () => {
    fetchMock.mockImplementation(async () => json({}));
    await post('/api/x', { a: 1 });
    expect(lastInit().body).toBe('{"a":1}');
    expect(lastInit().headers['Content-Type']).toBe('application/json');
    await post('/api/x', new FormData());
    expect(lastInit().headers['Content-Type']).toBeUndefined();
    await post('/api/x');
    expect(lastInit().body).toBeUndefined();
  });

  it('builds query skipping null/undefined', async () => {
    fetchMock.mockImplementation(async () => json({}));
    await get('/api/x', { query: { a: 1, b: undefined, c: 'x y', d: null } });
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/x?a=1&c=x+y');
  });

  it('rejects absolute cross-origin URL', async () => {
    await expect(get('https://evil.test/x')).rejects.toThrow(Error);
    await expect(get('https://evil.test/x')).rejects.not.toBeInstanceOf(ApiError);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('errors', () => {
  it('normalizes envelope', async () => {
    fetchMock.mockResolvedValue(envelope('INVALID_ARGUMENT', 400, { details: { fields: { name: 'wajib' } } }));
    const e: any = await get('/api/x').catch((x): any => x);
    expect(isApiError(e)).toBe(true);
    expect(e).toMatchObject({ status: 400, code: 'INVALID_ARGUMENT', message: 'pesan server', traceId: 't1' });
    expect(e.details).toEqual({ fields: { name: 'wajib' } });
  });

  it('missing details becomes null', async () => {
    fetchMock.mockResolvedValue(json({ error: { code: 'X', message: 'm' } }, 400));
    const e: any = await get('/api/x').catch((x): any => x);
    expect(e.details).toBeNull();
  });

  it('non-uniform body maps to INTERNAL with request id and locale message', async () => {
    fetchMock.mockImplementation(async () => new Response('<html>bad</html>', { status: 502, headers: { 'X-Request-Id': 'rid' } }));
    let e: any = await get('/api/x').catch((x): any => x);
    expect(e).toMatchObject({ status: 502, code: 'INTERNAL', traceId: 'rid', message: 'Terjadi kesalahan pada server' });
    setLocale('en');
    e = await get('/api/x').catch((x): any => x);
    expect(e.message).toBe('A server error occurred');
  });

  it('server message is verbatim regardless of locale', async () => {
    setLocale('en');
    fetchMock.mockResolvedValue(envelope('NOT_FOUND', 404));
    const e: any = await get('/api/x').catch((x): any => x);
    expect(e.message).toBe('pesan server');
  });

  it('network failure -> NETWORK', async () => {
    fetchMock.mockRejectedValue(new TypeError('fail'));
    const e: any = await get('/api/x').catch((x): any => x);
    expect(e).toMatchObject({ status: 0, code: 'NETWORK' });
  });

  it('abort propagates as AbortError', async () => {
    const ac = new AbortController();
    ac.abort();
    fetchMock.mockRejectedValue(new DOMException('aborted', 'AbortError'));
    const e: any = await get('/api/x', { signal: ac.signal }).catch((x): any => x);
    expect(isApiError(e)).toBe(false);
    expect(e.name).toBe('AbortError');
  });

  it('403 CSRF_REQUIRED does not redirect', async () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    fetchMock.mockResolvedValue(envelope('CSRF_REQUIRED', 403));
    const e: any = await post('/api/x').catch((x): any => x);
    expect(e.code).toBe('CSRF_REQUIRED');
    expect(h).not.toHaveBeenCalled();
  });
});

describe('401 handling', () => {
  it('calls handler with pathname+search then throws', async () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    history.pushState({}, '', '/projects/1?tab=key');
    fetchMock.mockResolvedValue(envelope('UNAUTHORIZED', 401));
    await expect(get('/api/x')).rejects.toBeInstanceOf(ApiError);
    expect(h).toHaveBeenCalledWith('/projects/1?tab=key');
  });
  it('skipAuthRedirect', async () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    fetchMock.mockResolvedValue(envelope('UNAUTHORIZED', 401));
    await get('/api/x', { skipAuthRedirect: true }).catch(() => {});
    expect(h).not.toHaveBeenCalled();
  });
  it('no loop on /login', async () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    history.pushState({}, '', '/login');
    fetchMock.mockResolvedValue(envelope('UNAUTHORIZED', 401));
    await get('/api/x').catch(() => {});
    expect(h).not.toHaveBeenCalled();
  });
  it('default handler uses location.assign', async () => {
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, pathname: '/a', search: '?b=1', assign });
    fetchMock.mockResolvedValue(envelope('UNAUTHORIZED', 401));
    await get('/api/x').catch(() => {});
    expect(assign).toHaveBeenCalledWith('/login?next=' + encodeURIComponent('/a?b=1'));
  });
});

describe('success', () => {
  it('validates mock health response with schema', async () => {
    fetchMock.mockResolvedValue(json({ status: 'ok' }));
    expect(await get('/healthz', { schema: HealthStatus })).toEqual({ status: 'ok' });
  });
  it('schema mismatch -> INVALID_RESPONSE with issues', async () => {
    fetchMock.mockResolvedValue(json({ status: 'weird' }));
    const e: any = await get('/healthz', { schema: HealthStatus }).catch((x): any => x);
    expect(e.code).toBe('INVALID_RESPONSE');
    expect(Array.isArray(e.details)).toBe(true);
  });
  it('invalid JSON on 2xx', async () => {
    fetchMock.mockResolvedValue(new Response('{oops', { status: 200 }));
    const e: any = await get('/x', { schema: z.any() }).catch((x): any => x);
    expect(e.code).toBe('INVALID_RESPONSE');
  });
  it('204 and empty 200 -> undefined', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    expect(await del('/api/x')).toBeUndefined();
    fetchMock.mockResolvedValue(new Response('', { status: 200 }));
    expect(await get('/api/x')).toBeUndefined();
  });
});

describe('toApiError', () => {
  it('maps a 409 envelope to code, details and traceId', () => {
    const e = toApiError(
      409,
      '{"error":{"code":"CONFLICT","message":"m","details":{"reason":"version_conflict"}},"trace_id":"t"}',
    );
    expect(e).toBeInstanceOf(ApiError);
    expect(e.status).toBe(409);
    expect(e.code).toBe('CONFLICT');
    expect(e.message).toBe('m');
    expect(e.details).toEqual({ reason: 'version_conflict' });
    expect(e.traceId).toBe('t');
  });
  it('falls back to the request id when the body has no trace_id', () => {
    const e = toApiError(500, '{"error":{"code":"INTERNAL","message":"m"}}', 'req-9');
    expect(e.traceId).toBe('req-9');
    expect(e.details).toBeNull();
  });
  it('maps a body that is not JSON to INTERNAL', () => {
    const e = toApiError(502, '<html>bad gateway</html>', 'req-1');
    expect(e.code).toBe('INTERNAL');
    expect(e.status).toBe(502);
    expect(e.traceId).toBe('req-1');
  });
  it('maps JSON that is not an envelope to INTERNAL', () => {
    expect(toApiError(400, '{"foo":1}').code).toBe('INTERNAL');
  });
});

describe('notifyUnauthorized', () => {
  it('calls the registered handler with the current path and query', () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    history.pushState({}, '', '/projects/p1/knowledge?path=a.md');
    notifyUnauthorized();
    expect(h).toHaveBeenCalledWith('/projects/p1/knowledge?path=a.md');
  });
  it('does nothing on the login page', () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    history.pushState({}, '', '/login');
    notifyUnauthorized();
    expect(h).not.toHaveBeenCalled();
  });
});

describe('getBlob', () => {
  it('returns the blob and content type without CSRF header', async () => {
    fetchMock.mockResolvedValue(new Response('openapi: 3.0.3', { status: 200, headers: { 'content-type': 'application/yaml' } }));
    const r = await getBlob('/api/x/raw');
    expect(await r.blob.text()).toBe('openapi: 3.0.3');
    expect(r.contentType).toBe('application/yaml');
    expect(lastInit().headers).not.toHaveProperty('X-Requested-With');
  });
  it('404 envelope -> ApiError with server message', async () => {
    fetchMock.mockResolvedValue(envelope('NOT_FOUND', 404));
    await expect(getBlob('/api/x/raw')).rejects.toMatchObject({ status: 404, message: 'pesan server' });
  });
  it('401 calls the unauthorized handler', async () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    fetchMock.mockResolvedValue(envelope('UNAUTHENTICATED', 401));
    await expect(getBlob('/api/x/raw')).rejects.toBeInstanceOf(ApiError);
    expect(h).toHaveBeenCalled();
  });
  it('network failure -> NETWORK', async () => {
    fetchMock.mockRejectedValue(new TypeError('fail'));
    await expect(getBlob('/api/x/raw')).rejects.toMatchObject({ code: 'NETWORK' });
  });
});

describe('openStream', () => {
  it('sends SSE headers, CSRF, JSON body and same-origin credentials', async () => {
    fetchMock.mockResolvedValue(new Response('', { status: 200 }));
    await openStream('POST', '/api/x/run', { body: { a: 1 } });
    const init = lastInit();
    expect(init.headers.Accept).toBe('text/event-stream');
    expect(init.headers['X-Requested-With']).toBe('grimoire');
    expect(init.headers['Content-Type']).toBe('application/json');
    expect(init.credentials).toBe('same-origin');
    expect(init.body).toBe('{"a":1}');
  });
  it('409 envelope -> ApiError with details', async () => {
    fetchMock.mockResolvedValue(envelope('SYNC_IN_PROGRESS', 409, { details: { kind: 'knowledge' } }));
    await expect(openStream('POST', '/api/x/run')).rejects.toMatchObject({ status: 409, code: 'SYNC_IN_PROGRESS', details: { kind: 'knowledge' } });
  });
  it('401 calls the unauthorized handler', async () => {
    const h = vi.fn();
    setUnauthorizedHandler(h);
    fetchMock.mockResolvedValue(envelope('UNAUTHENTICATED', 401));
    await expect(openStream('POST', '/api/x/run')).rejects.toBeInstanceOf(ApiError);
    expect(h).toHaveBeenCalled();
  });
  it('network failure -> NETWORK; abort is rethrown as is', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('fail'));
    await expect(openStream('POST', '/api/x/run')).rejects.toMatchObject({ code: 'NETWORK', status: 0 });
    const abort = new DOMException('x', 'AbortError');
    fetchMock.mockRejectedValueOnce(abort);
    await expect(openStream('POST', '/api/x/run')).rejects.toBe(abort);
  });
});
