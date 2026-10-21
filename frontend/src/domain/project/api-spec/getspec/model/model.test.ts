import { describe, expect, it, vi } from 'vitest';
import { mockDetail, mockDocument, mockEndpoints } from '../mocks/detail';
import { buildTagTree, endpointKey, getOperation, parseOpParam, UNTAGGED } from './operations';
import { deref, resolvePointer } from './refs';
import { filterEndpoints } from './search';
import { schemaChildren, typeLabel } from './schema';

describe('refs', () => {
  const doc = { a: { 'x/y': { '~z': [10, 20] } }, components: { schemas: { A: { $ref: '#/components/schemas/B' }, B: { type: 'string' }, L1: { $ref: '#/components/schemas/L2' }, L2: { $ref: '#/components/schemas/L1' } } } };
  it('resolves pointers with escapes and array indexes', () => {
    expect(resolvePointer(doc, '#/a/x~1y/~0z/1')).toBe(20);
    expect(resolvePointer(doc, '#')).toBe(doc);
    expect(resolvePointer(doc, '#/nope')).toBeUndefined();
  });
  it('follows chains, flags circular and unresolved, never fetches', () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal('fetch', fetchSpy);
    expect(deref(doc, { $ref: '#/components/schemas/A' })).toMatchObject({ node: { type: 'string' }, ref: '#/components/schemas/A' });
    expect(deref(doc, { $ref: '#/components/schemas/L1' }).circular).toBe(true);
    expect(deref(doc, { $ref: '#/missing' }).unresolved).toBe(true);
    expect(deref(doc, { $ref: 'http://x/y.yaml#/X' }).unresolved).toBe(true);
    expect(fetchSpy).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
  it('merges sibling description (3.1)', () => {
    expect(deref(doc, { $ref: '#/components/schemas/B', description: 'x' }).node).toMatchObject({ type: 'string', description: 'x' });
  });
});

describe('operations', () => {
  it('buildTagTree: doc.tags order, multi-tag, untagged last, path then method', () => {
    const tree = buildTagTree(mockDocument, mockEndpoints);
    expect(tree.map((g) => g.name)).toEqual(['refunds', 'payments', UNTAGGED]);
    expect(tree[1]!.endpoints.map((e) => endpointKey(e.method, e.path))).toEqual([
      'GET /payments',
      'POST /payments',
      'GET /payments/{id}',
      'POST /payments/{id}/refund',
    ]);
    expect(tree[0]!.endpoints).toHaveLength(1);
    expect(tree[0]!.description).toBe('Pengembalian dana');
  });
  it('parseOpParam', () => {
    expect(parseOpParam('POST /payments/{id}/refund')).toEqual({ method: 'POST', path: '/payments/{id}/refund' });
    expect(parseOpParam('FOO /x')).toBeNull();
    expect(parseOpParam('GET payments')).toBeNull();
    expect(parseOpParam(null)).toBeNull();
  });
  it('getOperation merges path-level params and falls back for servers/security', () => {
    const info = getOperation(mockDocument, 'GET', '/payments')!;
    expect(info.params.map((p) => p.name).sort()).toEqual(['X-Merchant', 'limit']);
    expect(info.servers).toHaveLength(1);
    expect(info.security).toBeNull();
    expect(getOperation(mockDocument, 'POST', '/payments')!.security).toEqual([{ bearer: [] }]);
    expect(getOperation(mockDocument, 'DELETE', '/payments')).toBeNull();
  });
  it('operation overrides path param with same name and location', () => {
    const doc = { paths: { '/x': { parameters: [{ name: 'a', in: 'query', description: 'path' }], get: { parameters: [{ name: 'a', in: 'query', description: 'op' }] } } } };
    expect(getOperation(doc, 'GET', '/x')!.params).toEqual([{ name: 'a', in: 'query', description: 'op' }]);
  });
  it('tolerates unexpected shapes', () => {
    expect(getOperation({ paths: 5 }, 'GET', '/x')).toBeNull();
    expect(buildTagTree({ tags: 5 }, mockEndpoints).length).toBeGreaterThan(0);
  });
});

describe('schema', () => {
  it('typeLabel normalizes 3.0 nullable and 3.1 type arrays', () => {
    expect(typeLabel({ type: 'string', nullable: true })).toBe('string | null');
    expect(typeLabel({ type: ['string', 'null'] })).toBe('string | null');
    expect(typeLabel({ type: 'array', items: { $ref: '#/components/schemas/Pet' } })).toBe('array<Pet>');
    expect(typeLabel({ type: 'string', format: 'date-time' })).toBe('string (date-time)');
    expect(typeLabel({ oneOf: [{ type: 'string' }, { type: 'integer' }] })).toBe('oneOf<string, integer>');
    expect(typeLabel({})).toBe('any');
    expect(typeLabel(5)).toBe('any');
  });
  it('schemaChildren marks required/readOnly and circular refs; odd shapes do not throw', () => {
    const doc = mockDetail().document;
    const rows = schemaChildren(doc, { $ref: '#/components/schemas/Payment' });
    expect(rows.map((r) => r.name)).toEqual(['id', 'amount', 'note', 'parent']);
    expect(rows[0]).toMatchObject({ required: true, readOnly: true });
    expect(rows[3]).toMatchObject({ circular: true });
    expect(schemaChildren({}, { properties: 5, required: 'x' })).toEqual([]);
  });
});

describe('search', () => {
  it('matches all tokens case-insensitively across fields', () => {
    expect(filterEndpoints(mockEndpoints, '')).toHaveLength(mockEndpoints.length);
    expect(filterEndpoints(mockEndpoints, 'POST refund').map((e) => e.operation_id)).toEqual(['refundPayment']);
    expect(filterEndpoints(mockEndpoints, 'LISTPAYMENTS')).toHaveLength(1);
    expect(filterEndpoints(mockEndpoints, 'tidak-ada')).toHaveLength(0);
  });
});
