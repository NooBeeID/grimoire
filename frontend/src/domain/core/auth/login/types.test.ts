import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { toLoginError } from './types';

const err = (code: string, status: number, details?: unknown) => new ApiError({ status, code, message: 'pesan', details, traceId: 'tr' });

describe('toLoginError', () => {
  it('maps INVALID_CREDENTIALS', () => expect(toLoginError(err('INVALID_CREDENTIALS', 401))).toEqual({ kind: 'invalid' }));
  it('maps ACCOUNT_LOCKED with and without retry', () => {
    expect(toLoginError(err('ACCOUNT_LOCKED', 429, { retry_after_seconds: 812 }))).toEqual({ kind: 'locked', retryAfterSeconds: 812 });
    expect(toLoginError(err('ACCOUNT_LOCKED', 429))).toEqual({ kind: 'locked', retryAfterSeconds: null });
  });
  it('maps RATE_LIMITED', () => {
    expect(toLoginError(err('RATE_LIMITED', 429, { retry_after_seconds: 3 }))).toEqual({ kind: 'rateLimited', retryAfterSeconds: 3 });
  });
  it('maps INVALID_ARGUMENT fields, else generic', () => {
    expect(toLoginError(err('INVALID_ARGUMENT', 400, { fields: { email: 'wajib' } }))).toEqual({ kind: 'fields', fields: { email: 'wajib' } });
    expect(toLoginError(err('INVALID_ARGUMENT', 400))).toMatchObject({ kind: 'generic' });
  });
  it('maps others to generic with trace id', () => {
    expect(toLoginError(err('INTERNAL', 500))).toEqual({ kind: 'generic', message: 'pesan', traceId: 'tr' });
    expect(toLoginError(new Error('x'))).toMatchObject({ kind: 'generic' });
  });
});
