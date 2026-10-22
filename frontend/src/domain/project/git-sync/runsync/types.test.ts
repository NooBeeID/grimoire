import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { mockConfig, mockKnowledgeReport } from '../base';
import { RunSyncResponseSchema, toSyncError } from './types';

const api = (code: string, details: unknown = null, status = 422) => new ApiError({ status, code, message: 'pesan', details, traceId: 'TR-1' });

describe('toSyncError', () => {
  it('INVALID_REPO_URL with details.field', () => {
    expect(toSyncError(api('INVALID_REPO_URL', { field: 'branch' }, 400))).toMatchObject({ code: 'INVALID_REPO_URL', field: 'branch', traceId: 'TR-1' });
  });
  it('CREDENTIAL_MISSING falls back to the default settings path', () => {
    expect(toSyncError(api('CREDENTIAL_MISSING', { host: 'github.com' })).settingsPath).toBe('/profile#token-git');
    expect(toSyncError(api('CREDENTIAL_MISSING', { settings_path: '/profile#x' })).settingsPath).toBe('/profile#x');
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
