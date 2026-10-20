import { describe, expect, it } from 'vitest';
import { ErrorEnvelope, HealthStatus, fieldErrorsOf, reasonOf, retryAfterOf } from './error';

describe('ErrorEnvelope', () => {
  it.each([null, { a: 1 }, [1, 2]])('accepts details %j', (details) => {
    expect(ErrorEnvelope.safeParse({ error: { code: 'X', message: 'm', details }, trace_id: 't' }).success).toBe(true);
  });
  it('accepts missing trace_id', () => {
    expect(ErrorEnvelope.safeParse({ error: { code: 'X', message: 'm' } }).success).toBe(true);
  });
  it('rejects empty code', () => {
    expect(ErrorEnvelope.safeParse({ error: { code: '', message: 'm' } }).success).toBe(false);
  });
  it('rejects non-object', () => {
    expect(ErrorEnvelope.safeParse('x').success).toBe(false);
  });
});

describe('HealthStatus', () => {
  it('validates mock health responses', () => {
    expect(HealthStatus.safeParse({ status: 'ok' }).success).toBe(true);
    expect(HealthStatus.safeParse({ status: 'unavailable' }).success).toBe(true);
    expect(HealthStatus.safeParse({ status: 'down' }).success).toBe(false);
  });
});

describe('fieldErrorsOf', () => {
  it('extracts fields', () => {
    expect(fieldErrorsOf({ fields: { name: 'wajib' } })).toEqual({ name: 'wajib' });
  });
  it('returns empty for other input', () => {
    expect(fieldErrorsOf(null)).toEqual({});
    expect(fieldErrorsOf({ fields: 1 })).toEqual({});
  });
});

describe('retryAfterOf / reasonOf', () => {
  it('reads retry_after_seconds', () => {
    expect(retryAfterOf({ retry_after_seconds: 812 })).toBe(812);
    expect(retryAfterOf({ retry_after_seconds: -1 })).toBeNull();
    expect(retryAfterOf(null)).toBeNull();
  });
  it('reads reason', () => {
    expect(reasonOf({ reason: 'LAST_SUPER_ADMIN' })).toBe('LAST_SUPER_ADMIN');
    expect(reasonOf({ reason: 1 })).toBeNull();
    expect(reasonOf(undefined)).toBeNull();
  });
});
