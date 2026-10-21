import { describe, expect, it } from 'vitest';
import { mockDetail } from './mocks/detail-3.0';
import { mockDetailSwagger2 } from './mocks/detail-swagger2';
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

describe('ApiSpecDetailSchema Swagger 2.0', () => {
  const info = { title: 't', version: '1' };
  const parse = (document: unknown) => ApiSpecDetailSchema.safeParse({ ...mockDetailSwagger2(), document }).success;
  it('menerima swagger "2.0" dan angka 2', () => {
    expect(parse({ swagger: '2.0', info })).toBe(true);
    expect(parse({ swagger: 2, info })).toBe(true);
  });
  it('menolak versi lain atau info tidak lengkap', () => {
    expect(parse({ swagger: '1.2', info })).toBe(false);
    expect(parse({ swagger: '2.0.0', info })).toBe(false);
    expect(parse({ swagger: null, info })).toBe(false);
    expect(parse({ swagger: '2.0', info: { version: '1' } })).toBe(false);
  });
  it('dokumen ber-openapi dan swagger lolos lewat cabang 3.x', () => {
    expect(parse({ openapi: '3.0.3', swagger: 'x', info })).toBe(true);
  });
});
