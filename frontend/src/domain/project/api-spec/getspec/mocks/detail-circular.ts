// Skema sirkular (Node.children[] -> Node) dan rantai ref yang saling menunjuk.
import { mockDetail } from './detail-3.0';
import type { ApiSpecDetail, ApiSpecEndpoint } from '../types';

export const mockEndpointsCircular: ApiSpecEndpoint[] = [
  { method: 'GET', path: '/tree', operation_id: 'getTree', summary: 'Pohon', tags: [], deprecated: false },
];

export const mockDocumentCircular = {
  openapi: '3.0.3',
  info: { title: 'Tree API', version: '1' },
  paths: {
    '/tree': {
      get: { summary: 'Pohon', responses: { '200': { description: 'OK', content: { 'application/json': { schema: { $ref: '#/components/schemas/Node' } } } } } },
    },
  },
  components: {
    schemas: {
      Node: { type: 'object', properties: { name: { type: 'string' }, children: { type: 'array', items: { $ref: '#/components/schemas/Node' } } } },
      A: { $ref: '#/components/schemas/B' },
      B: { $ref: '#/components/schemas/A' },
    },
  },
};

export const mockDetailCircular = (): ApiSpecDetail => mockDetail({ document: mockDocumentCircular, endpoints: mockEndpointsCircular });
