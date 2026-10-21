import { describe, expect, it } from 'vitest';
import { mockDocument, mockEndpoints } from '../mocks/detail-3.0';
import { mockDocumentWebhooks } from '../mocks/detail-webhooks';
import { buildTagTree, endpointKey, getOperation, getWebhookOperations, parseOpParam, UNTAGGED } from './operations';

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
    expect(getOperation(mockDocument, 'POST', '/payments')!.servers).toEqual([{ url: 'https://sandbox.noobee.id/payment' }]);
    expect(getOperation(mockDocument, 'DELETE', '/payments')).toBeNull();
  });
  it('operation overrides path param with same name and location, including via $ref', () => {
    const doc = {
      components: { parameters: { A: { name: 'a', in: 'query', description: 'op-ref' } } },
      paths: {
        '/x': { parameters: [{ name: 'a', in: 'query', description: 'path' }], get: { parameters: [{ name: 'a', in: 'query', description: 'op' }] } },
        '/y': { parameters: [{ name: 'a', in: 'query', description: 'path' }], get: { parameters: [{ $ref: '#/components/parameters/A' }] } },
      },
    };
    expect(getOperation(doc, 'GET', '/x')!.params).toEqual([{ name: 'a', in: 'query', description: 'op' }]);
    expect(getOperation(doc, 'GET', '/y')!.params).toEqual([{ name: 'a', in: 'query', description: 'op-ref' }]);
  });
  it('resolves path item $ref', () => {
    const doc = { components: { pathItems: { P: { get: { summary: 'via ref' } } } }, paths: { '/p': { $ref: '#/components/pathItems/P' } } };
    expect(getOperation(doc, 'GET', '/p')!.operation.summary).toBe('via ref');
  });
  it('servers fall back operation > path > root > []; security [] differs from undefined', () => {
    const doc = {
      servers: [{ url: 'https://root' }],
      security: [{ k: [] }],
      paths: {
        '/a': { servers: [{ url: 'https://path' }], get: { servers: [{ url: 'https://op' }], security: [] } },
        '/b': { servers: [{ url: 'https://path' }], get: {} },
        '/c': { get: {} },
      },
    };
    expect(getOperation(doc, 'GET', '/a')!.servers).toEqual([{ url: 'https://op' }]);
    expect(getOperation(doc, 'GET', '/b')!.servers).toEqual([{ url: 'https://path' }]);
    expect(getOperation(doc, 'GET', '/c')!.servers).toEqual([{ url: 'https://root' }]);
    expect(getOperation({ paths: { '/d': { get: {} } } }, 'GET', '/d')!.servers).toEqual([]);
    expect(getOperation(doc, 'GET', '/a')!.security).toEqual([]);
    expect(getOperation(doc, 'GET', '/c')!.security).toEqual([{ k: [] }]);
  });
  it('tolerates unexpected shapes', () => {
    expect(getOperation({ paths: 5 }, 'GET', '/x')).toBeNull();
    expect(buildTagTree({ tags: 5 }, mockEndpoints).length).toBeGreaterThan(0);
    expect(getWebhookOperations({ webhooks: 5 })).toEqual([]);
  });
  it('getWebhookOperations lists one entry per method', () => {
    const ops = getWebhookOperations(mockDocumentWebhooks);
    expect(ops.map((o) => `${o.method} ${o.name}`)).toEqual(['POST paymentCreated']);
    expect(ops[0]!.info.operation.summary).toBe('Pembayaran dibuat');
  });
});
