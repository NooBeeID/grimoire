import { describe, expect, it } from 'vitest';
import { mockDocumentCircular } from '../mocks/detail-circular';
import { mockDetail } from '../mocks/detail-3.0';
import { schemaChildren, typeLabel } from './schema';

describe('typeLabel', () => {
  it('normalizes 3.0 nullable and 3.1 type arrays', () => {
    expect(typeLabel({ type: 'string', nullable: true })).toBe('string | null');
    expect(typeLabel({ type: ['string', 'null'] })).toBe('string | null');
  });
  it('labels arrays, refs, formats, enums/const and compositions', () => {
    expect(typeLabel({ type: 'array', items: { $ref: '#/components/schemas/Pet' } })).toBe('array<Pet>');
    expect(typeLabel({ type: 'string', format: 'date-time' })).toBe('string (date-time)');
    expect(typeLabel({ oneOf: [{ type: 'string' }, { type: 'integer' }] })).toBe('oneOf<string, integer>');
    expect(typeLabel({ allOf: [{ $ref: '#/components/schemas/A' }] })).toBe('allOf<A>');
    expect(typeLabel({ const: 'x' })).toBe('"x"');
  });
  it('falls back to any', () => {
    expect(typeLabel({})).toBe('any');
    expect(typeLabel(5)).toBe('any');
  });
});

describe('schemaChildren', () => {
  it('marks required/readOnly and circular refs', () => {
    const rows = schemaChildren(mockDetail().document, { $ref: '#/components/schemas/Payment' });
    expect(rows.map((r) => r.name)).toEqual(['id', 'amount', 'note', 'parent']);
    expect(rows[0]).toMatchObject({ required: true, readOnly: true });
    expect(rows[3]).toMatchObject({ circular: true });
  });
  it('marks writeOnly and deprecated', () => {
    const rows = schemaChildren({}, { properties: { p: { type: 'string', writeOnly: true }, q: { type: 'string', deprecated: true } } });
    expect(rows[0]).toMatchObject({ writeOnly: true });
    expect(rows[1]).toMatchObject({ deprecated: true });
  });
  it('Node.children[] -> Node is circular one level below (T06-AC5)', () => {
    const doc = mockDocumentCircular;
    const root = { $ref: '#/components/schemas/Node' };
    const [, children] = schemaChildren(doc, root);
    expect(children!.name).toBe('children');
    const [items] = schemaChildren(doc, children!.schema, ['#/components/schemas/Node']);
    expect(items).toMatchObject({ name: '[]', circular: true });
  });
  it('flags unresolved refs', () => {
    expect(schemaChildren({}, { properties: { a: { $ref: '#/nope' } } })[0]).toMatchObject({ unresolved: true });
  });
  it('odd shapes do not throw', () => {
    expect(schemaChildren({}, { properties: 5, required: 'x' })).toEqual([]);
  });
});
