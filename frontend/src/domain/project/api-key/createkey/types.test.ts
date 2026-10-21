import { describe, expect, it } from 'vitest';
import { MOCK_RAW_KEY, mockCreated } from './mocks/created';
import { CreateKeyResponseSchema, validateName } from './types';

describe('validateName', () => {
  it.each([
    ['a', 'length'],
    ['ab', null],
    ['   ', 'length'],
    ['a\tb', 'control'],
  ])('%j -> %s', (raw, err) => expect(validateName(raw).error).toBe(err));
  it('trims', () => expect(validateName('  claude  ').name).toBe('claude'));
  it('counts runes, not UTF-16 units', () => {
    expect(validateName(String.fromCodePoint(0x1f600).repeat(60)).error).toBeNull();
    expect(validateName(String.fromCodePoint(0x1f600).repeat(61)).error).toBe('length');
  });
});

describe('CreateKeyResponseSchema', () => {
  it('accepts fixture and null mcp_url', () => {
    expect(CreateKeyResponseSchema.safeParse(mockCreated()).success).toBe(true);
    expect(CreateKeyResponseSchema.safeParse(mockCreated({ mcp_url: null })).success).toBe(true);
  });
  it('rejects malformed raw_key and mcp_url', () => {
    expect(CreateKeyResponseSchema.safeParse(mockCreated({ raw_key: MOCK_RAW_KEY + 'x' })).success).toBe(false);
    expect(CreateKeyResponseSchema.safeParse(mockCreated({ raw_key: 'GR_' + MOCK_RAW_KEY.slice(3) })).success).toBe(false);
    expect(CreateKeyResponseSchema.safeParse(mockCreated({ mcp_url: 'bukan url' })).success).toBe(false);
  });
});
