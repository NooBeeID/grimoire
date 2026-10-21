import { describe, expect, it } from 'vitest';
import { RenameResponseSchema, validateName } from './types';

describe('validateName', () => {
  it('ok / required / invalid', () => {
    expect(validateName('payment-v2')).toBe('ok');
    expect(validateName('')).toBe('required');
    for (const bad of ['Bad', 'a b', '..', 'x'.repeat(81)]) expect(validateName(bad)).toBe('invalid');
  });
  it('response schema requires spec', () => {
    expect(RenameResponseSchema.safeParse({}).success).toBe(false);
  });
});
