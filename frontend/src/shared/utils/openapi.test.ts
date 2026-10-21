import { describe, expect, it } from 'vitest';
import { isSwagger2Version, openApiLabel } from './openapi';

describe('openapi label', () => {
  it('"2.0" menjadi Swagger 2.0', () => expect(openApiLabel('2.0')).toBe('Swagger 2.0'));
  it('3.x menjadi OpenAPI <versi>', () => {
    expect(openApiLabel('3.0.3')).toBe('OpenAPI 3.0.3');
    expect(openApiLabel('3.1.0')).toBe('OpenAPI 3.1.0');
  });
  it('"2.0.0" bukan Swagger', () => expect(openApiLabel('2.0.0')).toBe('OpenAPI 2.0.0'));
  it('isSwagger2Version(null) false', () => {
    expect(isSwagger2Version(null)).toBe(false);
    expect(isSwagger2Version('2.0')).toBe(true);
  });
});
