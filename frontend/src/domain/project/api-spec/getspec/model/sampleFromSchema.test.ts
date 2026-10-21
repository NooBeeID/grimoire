import { describe, expect, it } from 'vitest';
import { sampleFromSchema } from './sampleFromSchema';

const doc = {
  components: {
    schemas: {
      Emp: {
        type: 'object',
        properties: {
          id: { type: 'string', format: 'uuid', readOnly: true },
          name: { type: 'string' },
          age: { type: 'integer' },
          role: { type: 'string', enum: ['admin', 'user'] },
          tags: { type: 'array', items: { type: 'string' } },
          boss: { $ref: '#/components/schemas/Emp' },
          secret: { type: 'string', writeOnly: true },
        },
      },
    },
  },
};

describe('sampleFromSchema', () => {
  it('request: skips readOnly, uses enum, nulls circular refs', () => {
    expect(sampleFromSchema(doc, { $ref: '#/components/schemas/Emp' }, 'request')).toEqual({
      name: 'string', age: 0, role: 'admin', tags: ['string'], boss: null, secret: 'string',
    });
  });
  it('response: skips writeOnly, keeps readOnly', () => {
    const r = sampleFromSchema(doc, { $ref: '#/components/schemas/Emp' }, 'response') as Record<string, unknown>;
    expect(r.id).toBe('3fa85f64-5717-4562-b3fc-2c963f66afa6');
    expect('secret' in r).toBe(false);
  });
  it('prefers example/default, merges allOf, takes first oneOf', () => {
    expect(sampleFromSchema({}, { type: 'string', example: 'x' })).toBe('x');
    expect(sampleFromSchema({}, { allOf: [{ type: 'object', properties: { a: { type: 'boolean' } } }, { type: 'object', properties: { b: { type: 'number' } } }] })).toEqual({ a: true, b: 0 });
    expect(sampleFromSchema({}, { oneOf: [{ type: 'string', format: 'email' }, { type: 'integer' }] })).toBe('user@example.com');
  });
  it('unknown shapes and unresolved refs yield null', () => {
    expect(sampleFromSchema({}, {})).toBeNull();
    expect(sampleFromSchema({}, { $ref: '#/nope' })).toBeNull();
  });
});
