import { ApiError, openStream } from '../../../../../shared/services/http';
import { createSseParser } from '../../../../../shared/services/sse';
import { get } from 'svelte/store';
import { t } from '../../../../../shared/stores/i18n';
import { RunSyncResponseSchema, parseSyncEvent, type RunSyncInput, type SyncEvent } from '../types';

// Idle timeout direset tiap byte (termasuk heartbeat); server mengirim heartbeat tiap 15 detik.
export const IDLE_TIMEOUT_MS = 45_000;

export async function runSyncStream(projectId: string, input: RunSyncInput, onEvent: (e: SyncEvent) => void, signal?: AbortSignal): Promise<void> {
  const body: Record<string, string> = {
    kind: input.kind,
    url: input.url.trim(),
    branch: input.branch.trim(),
    folder: input.folder.trim(),
  };
  if (input.kind === 'knowledge') body.target_prefix = (input.target_prefix ?? '').trim();

  // Gabungan sinyal pengguna dan idle timer; alasan abort dipertahankan (AbortError vs TimeoutError).
  const ctrl = new AbortController();
  let timer: ReturnType<typeof setTimeout> | null = null;
  const arm = () => {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => ctrl.abort(new DOMException('timeout', 'TimeoutError')), IDLE_TIMEOUT_MS);
  };
  const onAbort = () => ctrl.abort(signal?.reason ?? new DOMException('aborted', 'AbortError'));
  if (signal?.aborted) onAbort();
  else signal?.addEventListener('abort', onAbort, { once: true });
  arm();

  try {
    const res = await openStream('POST', `/api/projects/${encodeURIComponent(projectId)}/git-sync/run`, { body, signal: ctrl.signal });

    if (ctrl.signal.aborted) {
      void res.body?.cancel().catch(() => undefined);
      throw ctrl.signal.reason ?? new DOMException('aborted', 'AbortError');
    }

    if ((res.headers.get('Content-Type') ?? '').startsWith('application/json')) {
      const parsed = RunSyncResponseSchema.safeParse(await res.json().catch(() => null));
      if (!parsed.success) throw new ApiError({ status: res.status, code: 'INVALID_RESPONSE', message: get(t)('error.invalid_response'), details: parsed.error.issues });
      onEvent({ type: 'done', result: parsed.data });
      return;
    }
    if (!res.body) throw new ApiError({ status: 0, code: 'NETWORK', message: get(t)('error.network') });

    const reader = res.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let terminal = false;
    const parser = createSseParser((name, data) => {
      if (terminal) return;
      const ev = parseSyncEvent(name, data);
      if (!ev) return;
      if (ev.type === 'done' || ev.type === 'error') terminal = true;
      onEvent(ev);
    });
    // Abort (user/idle) harus membangunkan read() yang sedang menunggu.
    const stop = () => void reader.cancel().catch(() => undefined);
    ctrl.signal.addEventListener('abort', stop, { once: true });
    try {
      while (!terminal) {
        let chunk: ReadableStreamReadResult<Uint8Array>;
        try {
          chunk = await reader.read();
        } catch (e) {
          if (ctrl.signal.aborted) throw ctrl.signal.reason ?? e;
          throw new ApiError({ status: 0, code: 'NETWORK', message: get(t)('error.network') });
        }
        if (ctrl.signal.aborted) throw ctrl.signal.reason ?? new DOMException('aborted', 'AbortError');
        if (chunk.done) break;
        arm();
        parser.push(decoder.decode(chunk.value, { stream: true }));
      }
      if (!terminal) {
        parser.end();
        throw new ApiError({ status: 0, code: 'NETWORK', message: get(t)('error.network') });
      }
    } finally {
      ctrl.signal.removeEventListener('abort', stop);
      void reader.cancel().catch(() => undefined);
    }
  } finally {
    if (timer) clearTimeout(timer);
    signal?.removeEventListener('abort', onAbort);
  }
}
