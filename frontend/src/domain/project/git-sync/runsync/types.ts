import { z } from 'zod';
import { ApiError, isApiError } from '../../../../shared/services/http';
import { GitSyncConfigSchema, GitSyncReportSchema, SETTINGS_PATH, type Kind } from '../base';

export interface RunSyncInput {
  kind: Kind;
  url: string;
  branch: string;
  folder: string;
  target_prefix?: string;
}

export const RunSyncResponseSchema = z.object({ report: GitSyncReportSchema, config: GitSyncConfigSchema });
export type RunSyncResponse = z.infer<typeof RunSyncResponseSchema>;

export const SyncStartEventSchema = z.object({ total: z.number().int().min(0) });
export const SyncProgressEventSchema = z.object({ done: z.number().int().min(0), total: z.number().int().min(0), path: z.string() });
export const SyncWaitingEventSchema = z.object({ retry_after_seconds: z.number().int().min(0), reason: z.string() });
export const SyncSavingEventSchema = z.object({}).passthrough();
export const SyncErrorEventSchema = z.object({ code: z.string(), message: z.string(), details: z.unknown().optional().nullable() });

export type SyncField = 'url' | 'branch' | 'folder' | 'target_prefix';
const FIELDS: readonly string[] = ['url', 'branch', 'folder', 'target_prefix'];

export interface SyncError {
  code: string;
  message: string;
  status: number;
  traceId?: string;
  field?: SyncField;
  retryAfter?: number;
  settingsPath?: string;
  host?: string;
  folder?: string;
  limit?: { files: number; max_files: number; known_bytes: number; max_total_bytes: number };
}

const rec = (v: unknown): Record<string, unknown> => (typeof v === 'object' && v !== null ? (v as Record<string, unknown>) : {});
const num = (v: unknown): number | undefined => (typeof v === 'number' && Number.isFinite(v) ? v : undefined);
const str = (v: unknown): string | undefined => (typeof v === 'string' && v !== '' ? v : undefined);
const asField = (v: unknown): SyncField | undefined => (typeof v === 'string' && FIELDS.includes(v) ? (v as SyncField) : undefined);

function fieldOf(code: string, d: Record<string, unknown>): SyncField | undefined {
  const direct = asField(d.field);
  if (direct) return direct;
  const fromFields = Object.keys(rec(d.fields)).find((k) => FIELDS.includes(k));
  if (fromFields) return fromFields as SyncField;
  if (code === 'GIT_NOT_FOUND') return d.what === 'folder' ? 'folder' : d.what === 'branch' ? 'branch' : 'url';
  if (code === 'HOST_NOT_ALLOWED' || code === 'GIT_ACCESS_DENIED') return 'url';
  if (code === 'SYNC_LIMIT_EXCEEDED' || code === 'GIT_TREE_TOO_LARGE') return 'folder';
  return undefined;
}

export function toSyncError(e: unknown): SyncError {
  if (isApiError(e)) {
    const d = rec(e.details);
    const out: SyncError = { code: e.code, message: e.message, status: e.status, traceId: e.traceId };
    const field = fieldOf(e.code, d);
    if (field) out.field = field;
    const retry = num(d.retry_after_seconds);
    if (retry !== undefined) out.retryAfter = retry;
    if (e.code.startsWith('CREDENTIAL_') || e.code === 'GIT_TOKEN_REJECTED') out.settingsPath = str(d.settings_path) ?? SETTINGS_PATH;
    const host = str(d.host);
    if (host) out.host = host;
    const folder = str(d.folder);
    if (folder) out.folder = folder;
    const files = num(d.files);
    const maxFiles = num(d.max_files);
    if (files !== undefined && maxFiles !== undefined) {
      out.limit = { files, max_files: maxFiles, known_bytes: num(d.known_bytes) ?? 0, max_total_bytes: num(d.max_total_bytes) ?? 0 };
    }
    return out;
  }
  if (e instanceof DOMException && e.name === 'AbortError') return { code: 'ABORTED', message: '', status: 0 };
  if (e instanceof DOMException && e.name === 'TimeoutError') return { code: 'TIMEOUT', message: '', status: 0 };
  return { code: 'NETWORK', message: e instanceof ApiError ? e.message : '', status: 0 };
}

// State form dialog.
export interface FormValues {
  url: string;
  branch: string;
  folder: string;
  targetPrefix: string;
}
export interface FormTouched {
  branch: boolean;
  folder: boolean;
}

export type SyncEvent =
  | { type: 'start'; total: number }
  | { type: 'progress'; done: number; total: number; path: string }
  | { type: 'waiting'; retryAfter: number; reason: string }
  | { type: 'saving' }
  | { type: 'done'; result: RunSyncResponse }
  | { type: 'error'; error: SyncError };

export function toSyncErrorFromEvent(e: z.infer<typeof SyncErrorEventSchema>): SyncError {
  return toSyncError(new ApiError({ status: 200, code: e.code, message: e.message, details: e.details ?? null }));
}

// Nama tak dikenal, JSON rusak, atau skema gagal -> null (event dibuang).
export function parseSyncEvent(name: string, data: string): SyncEvent | null {
  let json: unknown;
  try {
    json = JSON.parse(data);
  } catch {
    return null;
  }
  switch (name) {
    case 'start': {
      const r = SyncStartEventSchema.safeParse(json);
      return r.success ? { type: 'start', total: r.data.total } : null;
    }
    case 'progress': {
      const r = SyncProgressEventSchema.safeParse(json);
      return r.success ? { type: 'progress', ...r.data } : null;
    }
    case 'waiting': {
      const r = SyncWaitingEventSchema.safeParse(json);
      return r.success ? { type: 'waiting', retryAfter: r.data.retry_after_seconds, reason: r.data.reason } : null;
    }
    case 'saving':
      return SyncSavingEventSchema.safeParse(json).success ? { type: 'saving' } : null;
    case 'done': {
      const r = RunSyncResponseSchema.safeParse(json);
      return r.success ? { type: 'done', result: r.data } : null;
    }
    case 'error': {
      const r = SyncErrorEventSchema.safeParse(json);
      return r.success ? { type: 'error', error: toSyncErrorFromEvent(r.data) } : null;
    }
    default:
      return null;
  }
}

// Koneksi putus atau idle timeout: hasil sync di server belum diketahui.
export function isStreamLost(e: SyncError): boolean {
  return (e.code === 'NETWORK' || e.code === 'TIMEOUT') && e.status === 0;
}
