import { get as getStore } from 'svelte/store';
import type { z } from 'zod';
import { t } from '../stores/i18n';
import { ErrorEnvelope } from '../validation/error';

export const CSRF_HEADER = 'X-Requested-With';
export const CSRF_VALUE = 'grimoire';

export class ApiError extends Error {
  override readonly name = 'ApiError';
  readonly status: number;
  readonly code: string;
  readonly details: unknown;
  readonly traceId?: string;
  constructor(i: { status: number; code: string; message: string; details?: unknown; traceId?: string }) {
    super(i.message);
    this.status = i.status;
    this.code = i.code;
    this.details = i.details ?? null;
    this.traceId = i.traceId;
  }
}

export function isApiError(e: unknown): e is ApiError {
  return e instanceof ApiError;
}

export interface RequestOptions<T> {
  body?: unknown;
  query?: Record<string, string | number | boolean | null | undefined>;
  schema?: z.ZodType<T>;
  signal?: AbortSignal;
  skipAuthRedirect?: boolean;
  headers?: Record<string, string>;
}

let unauthorizedHandler: ((nextPath: string) => void) | null = null;
export function setUnauthorizedHandler(fn: ((nextPath: string) => void) | null): void {
  unauthorizedHandler = fn;
}

const MUTATIONS = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);
const tr = (key: string) => getStore(t)(key);

function buildUrl(url: string, query?: RequestOptions<unknown>['query']): string {
  if (!url.startsWith('/') || url.startsWith('//')) {
    throw new Error(`http: URL harus relatif same-origin, diterima "${url}"`);
  }
  if (!query) return url;
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === null || v === undefined) continue;
    p.append(k, String(v));
  }
  const qs = p.toString();
  return qs ? `${url}${url.includes('?') ? '&' : '?'}${qs}` : url;
}

function isRawBody(b: unknown): b is FormData | Blob | URLSearchParams {
  return (
    (typeof FormData !== 'undefined' && b instanceof FormData) ||
    (typeof Blob !== 'undefined' && b instanceof Blob) ||
    (typeof URLSearchParams !== 'undefined' && b instanceof URLSearchParams)
  );
}

export function notifyUnauthorized(): void {
  const here = location.pathname;
  if (here === '/login') return;
  const next = here + location.search;
  if (unauthorizedHandler) unauthorizedHandler(next);
  else window.location.assign('/login?next=' + encodeURIComponent(next));
}

export function toApiError(status: number, bodyText: string, requestId?: string): ApiError {
  try {
    const env = ErrorEnvelope.safeParse(JSON.parse(bodyText));
    if (!env.success) throw new Error('not envelope');
    return new ApiError({
      status,
      code: env.data.error.code,
      message: env.data.error.message,
      details: env.data.error.details ?? null,
      traceId: env.data.trace_id ?? requestId,
    });
  } catch {
    return new ApiError({ status, code: 'INTERNAL', message: tr('error.internal'), traceId: requestId });
  }
}

export async function request<T = unknown>(method: string, url: string, o: RequestOptions<T> = {}): Promise<T> {
  const m = method.toUpperCase();
  const full = buildUrl(url, o.query);

  const headers: Record<string, string> = { Accept: 'application/json', ...o.headers };
  if (MUTATIONS.has(m)) headers[CSRF_HEADER] = CSRF_VALUE;

  let body: BodyInit | undefined;
  if (o.body !== undefined) {
    if (isRawBody(o.body)) body = o.body;
    else {
      body = JSON.stringify(o.body);
      headers['Content-Type'] = 'application/json';
    }
  }

  let res: Response;
  try {
    res = await fetch(full, { method: m, headers, body, credentials: 'same-origin', signal: o.signal });
  } catch (e) {
    if ((e instanceof DOMException && e.name === 'AbortError') || o.signal?.aborted) throw e;
    throw new ApiError({ status: 0, code: 'NETWORK', message: tr('error.network') });
  }

  const requestId = res.headers.get('X-Request-Id') ?? undefined;

  if (res.ok) {
    const text = res.status === 204 ? '' : await res.text();
    if (text === '') return undefined as T;
    let json: unknown;
    try {
      json = JSON.parse(text);
    } catch {
      throw new ApiError({ status: res.status, code: 'INVALID_RESPONSE', message: tr('error.invalid_response'), traceId: requestId });
    }
    if (!o.schema) return json as T;
    const parsed = o.schema.safeParse(json);
    if (!parsed.success) {
      throw new ApiError({
        status: res.status,
        code: 'INVALID_RESPONSE',
        message: tr('error.invalid_response'),
        details: parsed.error.issues,
        traceId: requestId,
      });
    }
    return parsed.data;
  }

  const text = await res.text().catch(() => '');
  const err = toApiError(res.status, text, requestId);

  if (res.status === 401 && !o.skipAuthRedirect) notifyUnauthorized();
  throw err;
}

export interface StreamOptions {
  body?: unknown;
  signal?: AbortSignal;
  headers?: Record<string, string>;
}

// Request yang membaca respons sebagai stream (SSE). 2xx dikembalikan mentah; non-2xx dipetakan ke ApiError.
export async function openStream(method: string, url: string, o: StreamOptions = {}): Promise<Response> {
  const m = method.toUpperCase();
  const full = buildUrl(url);
  const headers: Record<string, string> = { ...o.headers, Accept: 'text/event-stream' };
  if (MUTATIONS.has(m)) headers[CSRF_HEADER] = CSRF_VALUE;
  let body: BodyInit | undefined;
  if (o.body !== undefined) {
    body = JSON.stringify(o.body);
    headers['Content-Type'] = 'application/json';
  }
  let res: Response;
  try {
    res = await fetch(full, { method: m, headers, body, credentials: 'same-origin', signal: o.signal });
  } catch (e) {
    if (e instanceof DOMException && (e.name === 'AbortError' || e.name === 'TimeoutError')) throw e;
    if (o.signal?.aborted) throw e;
    throw new ApiError({ status: 0, code: 'NETWORK', message: tr('error.network') });
  }
  if (res.ok) return res;
  const requestId = res.headers.get('X-Request-Id') ?? undefined;
  const text = await res.text().catch(() => '');
  const err = toApiError(res.status, text, requestId);
  if (res.status === 401) notifyUnauthorized();
  throw err;
}

export interface BlobResult {
  blob: Blob;
  contentType: string | null;
}

// GET non-JSON (unduhan). Gagal -> ApiError dengan envelope dan handler 401 yang sama dengan request().
export async function getBlob(url: string, o: { signal?: AbortSignal; skipAuthRedirect?: boolean } = {}): Promise<BlobResult> {
  const full = buildUrl(url);
  let res: Response;
  try {
    res = await fetch(full, { method: 'GET', headers: { Accept: '*/*' }, credentials: 'same-origin', signal: o.signal });
  } catch (e) {
    if ((e instanceof DOMException && e.name === 'AbortError') || o.signal?.aborted) throw e;
    throw new ApiError({ status: 0, code: 'NETWORK', message: tr('error.network') });
  }
  const requestId = res.headers.get('X-Request-Id') ?? undefined;
  if (res.ok) return { blob: await res.blob(), contentType: res.headers.get('Content-Type') };
  const text = await res.text().catch(() => '');
  const err = toApiError(res.status, text, requestId);
  if (res.status === 401 && !o.skipAuthRedirect) notifyUnauthorized();
  throw err;
}

type NoBody<T> = Omit<RequestOptions<T>, 'body'>;
export const get = <T>(url: string, o?: NoBody<T>) => request<T>('GET', url, o);
export const post = <T>(url: string, body?: unknown, o?: RequestOptions<T>) => request<T>('POST', url, { ...o, body });
export const put = <T>(url: string, body?: unknown, o?: RequestOptions<T>) => request<T>('PUT', url, { ...o, body });
export const patch = <T>(url: string, body?: unknown, o?: RequestOptions<T>) => request<T>('PATCH', url, { ...o, body });
export const del = <T>(url: string, o?: RequestOptions<T>) => request<T>('DELETE', url, o);
