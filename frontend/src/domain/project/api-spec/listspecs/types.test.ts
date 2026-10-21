import { describe, expect, it } from 'vitest';
import { mockAuthSpec, mockSpec, mockSpecList } from './mocks/specs';
import { ApiSpecListSchema, ApiSpecSchema, formatBytes, openapiMajorMinor, sortSpecs } from './types';

describe('schemas', () => {
  it('accepts the mock list', () => {
    expect(ApiSpecListSchema.safeParse(mockSpecList()).success).toBe(true);
  });
  it('rejects negative endpoint_count and bad format', () => {
    expect(ApiSpecSchema.safeParse(mockSpec({ endpoint_count: -1 })).success).toBe(false);
    expect(ApiSpecSchema.safeParse({ ...mockSpec(), format: 'xml' }).success).toBe(false);
  });
});
describe('helpers', () => {
  it('sortSpecs sorts by name without mutating', () => {
    const input = [mockSpec(), mockAuthSpec()];
    expect(sortSpecs(input).map((s) => s.name)).toEqual(['auth', 'payment']);
    expect(input[0]!.name).toBe('payment');
  });
  it('openapiMajorMinor', () => {
    expect(openapiMajorMinor('3.1.0')).toBe('3.1');
    expect(openapiMajorMinor('3.0')).toBe('3.0');
    expect(openapiMajorMinor('x')).toBe('x');
  });
  it('formatBytes', () => {
    expect(formatBytes(512, 'id')).toBe('512 B');
    expect(formatBytes(18234, 'id')).toBe('17,8 KB');
    expect(formatBytes(18234, 'en')).toBe('17.8 KB');
  });
});
