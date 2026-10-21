import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../shared/services/http';
import { errorMessage, isNotFound, reasonMessageKey } from './errors';

const tr = (k: string, p?: Record<string, unknown>) => (p ? `${k}|${JSON.stringify(p)}` : k);
const api = (status: number, details: unknown = null, message = 'pesan server') =>
  new ApiError({ status, code: 'X', message, details });

describe('reasonMessageKey', () => {
  it('maps known reasons only', () => {
    expect(reasonMessageKey('path_traversal')).toBe('knowledge.reason.path_traversal');
    expect(reasonMessageKey('bogus')).toBeNull();
    expect(reasonMessageKey(null)).toBeNull();
  });
});

describe('errorMessage', () => {
  it('fills {max} from details.max_bytes for too_large', () => {
    const e = api(413, { reason: 'too_large', max_bytes: 1048576 });
    expect(errorMessage(e, tr)).toBe('knowledge.reason.too_large|{"max":"1 MB"}');
  });
  it('uses an empty {max} when max_bytes is absent', () => {
    expect(errorMessage(api(400, { reason: 'hidden' }), tr)).toBe('knowledge.reason.hidden|{"max":""}');
  });
  it('falls back to the server message for an unknown reason', () => {
    expect(errorMessage(api(400, { reason: 'something_new' }), tr)).toBe('pesan server');
  });
  it('falls back to the server message when there is no reason', () => {
    expect(errorMessage(api(500), tr)).toBe('pesan server');
  });
  it('maps a non-ApiError to error.internal', () => {
    expect(errorMessage(new Error('boom'), tr)).toBe('error.internal');
    expect(errorMessage('x', tr)).toBe('error.internal');
  });
});

describe('isNotFound', () => {
  it('is true only for ApiError with status 404', () => {
    expect(isNotFound(api(404))).toBe(true);
    expect(isNotFound(api(409))).toBe(false);
    expect(isNotFound(new Error('x'))).toBe(false);
  });
});
