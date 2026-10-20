import { describe, expect, it } from 'vitest';
import { ErrorEnvelope, HealthStatus, fieldErrorsOf } from './error';

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
