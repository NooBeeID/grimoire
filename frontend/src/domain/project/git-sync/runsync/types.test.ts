import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { mockConfig, mockKnowledgeReport } from '../base';
import {
  RunSyncResponseSchema,
  SyncProgressEventSchema,
  SyncStartEventSchema,
  SyncWaitingEventSchema,
  isStreamLost,
  parseSyncEvent,
  toSyncError,
  toSyncErrorFromEvent,
} from './types';

const api = (code: string, details: unknown = null, status = 422) => new ApiError({ status, code, message: 'pesan', details, traceId: 'TR-1' });

describe('toSyncError', () => {
  it('INVALID_REPO_URL with details.field', () => {
    expect(toSyncError(api('INVALID_REPO_URL', { field: 'branch' }, 400))).toMatchObject({ code: 'INVALID_REPO_URL', field: 'branch', traceId: 'TR-1' });
  });
  it('CREDENTIAL_MISSING falls back to the default settings path', () => {
    expect(toSyncError(api('CREDENTIAL_MISSING', { host: 'github.com' })).settingsPath).toBe('/settings#token-git');
    expect(toSyncError(api('CREDENTIAL_MISSING', { settings_path: '/settings#x' })).settingsPath).toBe('/settings#x');
  });
  it('RATE_LIMITED retry_after_seconds', () => {
    expect(toSyncError(api('RATE_LIMITED', { retry_after_seconds: 60 }, 429)).retryAfter).toBe(60);
  });
  it('SYNC_LIMIT_EXCEEDED -> limit and folder field', () => {
    const r = toSyncError(api('SYNC_LIMIT_EXCEEDED', { files: 742, max_files: 500, known_bytes: 3145728, max_total_bytes: 52428800 }));
    expect(r.limit).toEqual({ files: 742, max_files: 500, known_bytes: 3145728, max_total_bytes: 52428800 });
    expect(r.field).toBe('folder');
  });
  it('GIT_NOT_FOUND what=folder -> folder', () => {
    expect(toSyncError(api('GIT_NOT_FOUND', { what: 'folder', folder: 'tidak-ada' }, 404))).toMatchObject({ field: 'folder', folder: 'tidak-ada' });
    expect(toSyncError(api('GIT_NOT_FOUND', { what: 'repo' }, 404)).field).toBe('url');
  });
  it('details.fields picks the first known field', () => {
    expect(toSyncError(api('INVALID_ARGUMENT', { fields: { target_prefix: 'x' } }, 400)).field).toBe('target_prefix');
  });
  it('AbortError -> ABORTED, TimeoutError -> TIMEOUT, other -> NETWORK', () => {
    expect(toSyncError(new DOMException('x', 'AbortError')).code).toBe('ABORTED');
    expect(toSyncError(new DOMException('x', 'TimeoutError')).code).toBe('TIMEOUT');
    expect(toSyncError(new Error('boom')).code).toBe('NETWORK');
  });
});

describe('RunSyncResponseSchema', () => {
  it('accepts report + config', () => {
    expect(RunSyncResponseSchema.safeParse({ report: mockKnowledgeReport(), config: mockConfig({ last_result: null }) }).success).toBe(true);
  });
});

describe('event schemas', () => {
  it('validates start/progress/waiting', () => {
    expect(SyncStartEventSchema.safeParse({ total: 3 }).success).toBe(true);
    expect(SyncStartEventSchema.safeParse({ total: -1 }).success).toBe(false);
    expect(SyncProgressEventSchema.safeParse({ done: 1, total: 3, path: 'a' }).success).toBe(true);
    expect(SyncProgressEventSchema.safeParse({ done: '1', total: 3, path: 'a' }).success).toBe(false);
    expect(SyncWaitingEventSchema.safeParse({ retry_after_seconds: 38, reason: 'rate_limit_exhausted' }).success).toBe(true);
    expect(SyncWaitingEventSchema.safeParse({ retry_after_seconds: 1.5, reason: 'x' }).success).toBe(false);
  });
});

describe('parseSyncEvent', () => {
  it('maps known events and drops unknown, broken, or invalid ones', () => {
    expect(parseSyncEvent('start', '{"total":3}')).toEqual({ type: 'start', total: 3 });
    expect(parseSyncEvent('waiting', '{"retry_after_seconds":38,"reason":"r"}')).toEqual({ type: 'waiting', retryAfter: 38, reason: 'r' });
    expect(parseSyncEvent('saving', '{}')).toEqual({ type: 'saving' });
    expect(parseSyncEvent('unknown', '{}')).toBeNull();
    expect(parseSyncEvent('start', '{oops')).toBeNull();
    expect(parseSyncEvent('progress', '{"done":"1"}')).toBeNull();
  });
  it('done carries a valid RunSyncResponse', () => {
    const e = parseSyncEvent('done', JSON.stringify({ report: mockKnowledgeReport(), config: mockConfig() }));
    expect(e?.type).toBe('done');
    expect(parseSyncEvent('done', '{"report":{}}')).toBeNull();
  });
  it('error event becomes a SyncError', () => {
    const e = parseSyncEvent('error', '{"code":"GIT_NOT_FOUND","message":"x","details":null}');
    expect(e).toMatchObject({ type: 'error', error: { code: 'GIT_NOT_FOUND' } });
  });
});

describe('toSyncErrorFromEvent', () => {
  it('RATE_LIMITED carries retryAfter', () => {
    expect(toSyncErrorFromEvent({ code: 'RATE_LIMITED', message: 'm', details: { retry_after_seconds: 1712 } })).toMatchObject({ code: 'RATE_LIMITED', retryAfter: 1712 });
  });
  it('SYNC_LIMIT_EXCEEDED carries limit and folder field; null details are safe', () => {
    const e = toSyncErrorFromEvent({ code: 'SYNC_LIMIT_EXCEEDED', message: 'm', details: { files: 900, max_files: 500 } });
    expect(e.limit).toMatchObject({ files: 900, max_files: 500 });
    expect(e.field).toBe('folder');
    expect(toSyncErrorFromEvent({ code: 'INTERNAL', message: 'm', details: null })).toMatchObject({ code: 'INTERNAL' });
  });
});

describe('isStreamLost', () => {
  it('NETWORK/TIMEOUT with status 0 only', () => {
    expect(isStreamLost({ code: 'NETWORK', message: '', status: 0 })).toBe(true);
    expect(isStreamLost({ code: 'TIMEOUT', message: '', status: 0 })).toBe(true);
    expect(isStreamLost({ code: 'RATE_LIMITED', message: '', status: 200 })).toBe(false);
    expect(isStreamLost({ code: 'TIMEOUT', message: '', status: 200 })).toBe(false);
  });
});
