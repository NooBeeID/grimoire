import { post } from '../../../../../shared/services/http';
import type { RunSyncInput, RunSyncResponse } from '../types';
import { RunSyncResponseSchema } from '../types';

// Timeout klien > GIT_SYNC_TIMEOUT server (120 s).
export const CLIENT_TIMEOUT_MS = 130_000;

export async function runSync(projectId: string, input: RunSyncInput, signal?: AbortSignal): Promise<RunSyncResponse> {
  const body: Record<string, string> = {
    kind: input.kind,
    url: input.url.trim(),
    branch: input.branch.trim(),
    folder: input.folder.trim(),
  };
  if (input.kind === 'knowledge') body.target_prefix = (input.target_prefix ?? '').trim();

  // Gabungan sinyal pengguna dan timeout; alasan abort dipertahankan (AbortError vs TimeoutError).
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(new DOMException('timeout', 'TimeoutError')), CLIENT_TIMEOUT_MS);
  const onAbort = () => ctrl.abort(signal?.reason ?? new DOMException('aborted', 'AbortError'));
  if (signal?.aborted) onAbort();
  else signal?.addEventListener('abort', onAbort, { once: true });
  try {
    return await post(`/api/projects/${encodeURIComponent(projectId)}/git-sync/run`, body, { schema: RunSyncResponseSchema, signal: ctrl.signal });
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener('abort', onAbort);
  }
}
