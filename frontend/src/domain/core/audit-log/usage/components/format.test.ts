import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../../../shared/services/http';
import { sectionError } from './format';

describe('sectionError', () => {
  it('maps unavailable to ApiError with message', () => {
    const e = sectionError('unavailable', 'gagal');
    expect(e).toBeInstanceOf(ApiError);
    expect(e).toMatchObject({ code: 'UNAVAILABLE', message: 'gagal' });
  });
  it('passes through null and ApiError', () => {
    const a = new ApiError({ status: 500, code: 'X', message: 'm' });
    expect(sectionError(null, 'm')).toBeNull();
    expect(sectionError(a, 'm')).toBe(a);
  });
});
