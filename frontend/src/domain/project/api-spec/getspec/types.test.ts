import { describe, expect, it } from 'vitest';
import { mockDetail } from './mocks/detail-3.0';
import { ApiSpecDetailSchema } from './types';

describe('ApiSpecDetailSchema', () => {
  it('accepts the mock and keeps extra document keys', () => {
    const r = ApiSpecDetailSchema.safeParse(mockDetail());
    expect(r.success).toBe(true);
    expect(r.success && (r.data.document as Record<string, unknown>).paths).toBeDefined();
  });
  it('rejects a document without openapi', () => {
    const d = mockDetail();
    expect(ApiSpecDetailSchema.safeParse({ ...d, document: { info: d.document.info } }).success).toBe(false);
  });
});
