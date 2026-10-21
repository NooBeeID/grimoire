import type { z } from 'zod';
import { get as getStore } from 'svelte/store';
import { t } from '../stores/i18n';
import { ApiError, CSRF_HEADER, CSRF_VALUE, notifyUnauthorized, toApiError } from './http';

export interface UploadProgress {
  loaded: number;
  total: number;
  phase: 'sending' | 'processing';
}

export interface MultipartOptions<T> {
  schema: z.ZodType<T>;
  signal?: AbortSignal;
  onProgress?: (p: UploadProgress) => void;
}

const tr = (key: string) => getStore(t)(key);

// Multipart dengan progres unggah (fetch tidak menyediakannya).
export function postMultipart<T>(url: string, form: FormData, o: MultipartOptions<T>): Promise<T> {
  if (!url.startsWith('/') || url.startsWith('//')) {
    return Promise.reject(new Error(`upload: URL harus relatif same-origin, diterima "${url}"`));
  }
  return new Promise<T>((resolve, reject) => {
    const abortError = () => new DOMException('Aborted', 'AbortError');
    if (o.signal?.aborted) return reject(abortError());

    const xhr = new XMLHttpRequest();
    let total = 0;
    const onAbort = () => xhr.abort();
    const done = () => o.signal?.removeEventListener('abort', onAbort);

    xhr.open('POST', url);
    xhr.withCredentials = true;
    xhr.setRequestHeader('Accept', 'application/json');
    xhr.setRequestHeader(CSRF_HEADER, CSRF_VALUE);

    xhr.upload.onprogress = (e) => {
      total = e.lengthComputable ? e.total : total;
      o.onProgress?.({ loaded: e.loaded, total: e.lengthComputable ? e.total : 0, phase: 'sending' });
    };
    xhr.upload.onload = () => o.onProgress?.({ loaded: total, total, phase: 'processing' });

    xhr.onload = () => {
      done();
      const requestId = xhr.getResponseHeader('X-Request-Id') ?? undefined;
      const text = xhr.responseText ?? '';
      if (xhr.status >= 200 && xhr.status < 300) {
        let json: unknown;
        try {
          json = JSON.parse(text);
        } catch {
          return reject(new ApiError({ status: xhr.status, code: 'INVALID_RESPONSE', message: tr('error.invalid_response'), traceId: requestId }));
        }
        const parsed = o.schema.safeParse(json);
        if (!parsed.success) {
          return reject(
            new ApiError({
              status: xhr.status,
              code: 'INVALID_RESPONSE',
              message: tr('error.invalid_response'),
              details: parsed.error.issues,
              traceId: requestId,
            }),
          );
        }
        return resolve(parsed.data);
      }
      const err = toApiError(xhr.status, text, requestId);
      if (xhr.status === 401) notifyUnauthorized();
      reject(err);
    };
    xhr.onerror = () => {
      done();
      reject(new ApiError({ status: 0, code: 'NETWORK', message: tr('error.network') }));
    };
    xhr.onabort = () => {
      done();
      reject(abortError());
    };

    o.signal?.addEventListener('abort', onAbort, { once: true });
    xhr.send(form);
  });
}
