// Spec OpenAPI 3.1: type berupa array, $ref dengan kunci saudara, skema keamanan OAuth2.
import { mockDetail } from './detail-3.0';
import type { ApiSpecDetail, ApiSpecEndpoint } from '../types';

export const mockEndpoints31: ApiSpecEndpoint[] = [
  { method: 'GET', path: '/pets/{id}', operation_id: 'getPet', summary: 'Detail hewan', tags: ['pets'], deprecated: false },
];

export const mockDocument31 = {
  openapi: '3.1.0',
  info: { title: 'Petstore', version: '2.0.0', summary: 'Toko hewan' },
  servers: [{ url: 'https://pets.example.com/v2', description: 'Produksi' }],
  security: [{ oauth: ['pets:read'] }],
  paths: {
    '/pets/{id}': {
      get: {
        operationId: 'getPet',
        summary: 'Detail hewan',
        tags: ['pets'],
        parameters: [{ name: 'id', in: 'path', required: true, schema: { type: 'string' }, example: 'p-1' }],
        responses: {
          '200': {
            description: 'OK',
            content: { 'application/json': { schema: { $ref: '#/components/schemas/Pet', description: 'Hewan' } } },
          },
        },
      },
    },
  },
  components: {
    schemas: {
      Pet: {
        type: 'object',
        required: ['id'],
        properties: { id: { type: 'string' }, nickname: { type: ['string', 'null'] }, tags: { type: 'array', items: { type: 'string' } } },
      },
    },
    securitySchemes: {
      oauth: {
        type: 'oauth2',
        flows: { authorizationCode: { authorizationUrl: 'https://auth.example.com/authorize', tokenUrl: 'https://auth.example.com/token', scopes: { 'pets:read': 'Baca hewan' } } },
      },
    },
  },
};

export const mockDetail31 = (): ApiSpecDetail => ({
  ...mockDetail({ document: mockDocument31, endpoints: mockEndpoints31 }),
  spec: { ...mockDetail().spec, name: 'petstore', title: 'Petstore', openapi_version: '3.1.0', api_version: '2.0.0', format: 'json', endpoint_count: 1 },
});
