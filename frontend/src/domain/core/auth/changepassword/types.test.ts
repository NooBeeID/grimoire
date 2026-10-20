import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { toChangePasswordError } from './types';

const err = (code: string, status: number, details?: unknown) => new ApiError({ status, code, message: 'pesan', details, traceId: 'tr' });

describe('toChangePasswordError', () => {
  it('maps fields', () => {
    expect(toChangePasswordError(err('INVALID_ARGUMENT', 400, { fields: { new_password: 'pendek' } }))).toEqual({ kind: 'fields', fields: { new_password: 'pendek' } });
  });
  it('maps conflict and rate limit', () => {
    expect(toChangePasswordError(err('CONFLICT', 409))).toEqual({ kind: 'conflict' });
    expect(toChangePasswordError(err('RATE_LIMITED', 429, { retry_after_seconds: 7 }))).toEqual({ kind: 'rateLimited', retryAfterSeconds: 7 });
  });
  it('falls back to generic', () => {
    expect(toChangePasswordError(err('INTERNAL', 500))).toEqual({ kind: 'generic', message: 'pesan', traceId: 'tr' });
    expect(toChangePasswordError(err('INVALID_ARGUMENT', 400))).toMatchObject({ kind: 'generic' });
    expect(toChangePasswordError('x')).toMatchObject({ kind: 'generic' });
  });
});
