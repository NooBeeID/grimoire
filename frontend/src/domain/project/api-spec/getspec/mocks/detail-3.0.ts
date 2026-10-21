// Data mock sesuai skema ApiSpecDetail (api/modules/project/api-spec.yml).
import type { ApiSpecDetail, ApiSpecEndpoint } from '../types';

export const mockEndpoints: ApiSpecEndpoint[] = [
  { method: 'POST', path: '/payments', operation_id: 'createPayment', summary: 'Buat pembayaran', tags: ['payments'], deprecated: false },
  { method: 'GET', path: '/payments', operation_id: 'listPayments', summary: 'Daftar pembayaran', tags: ['payments'], deprecated: false },
  { method: 'GET', path: '/payments/{id}', operation_id: 'getPayment', summary: 'Detail pembayaran', tags: ['payments'], deprecated: false },
  { method: 'POST', path: '/payments/{id}/refund', operation_id: 'refundPayment', summary: 'Kembalikan dana', tags: ['payments', 'refunds'], deprecated: true },
  { method: 'GET', path: '/health', operation_id: null, summary: 'Cek layanan', tags: [], deprecated: false },
];

export const mockDocument = {
  openapi: '3.0.3',
  info: { title: 'Payment API', version: '1.4.0', description: 'Pembayaran antar-merchant.' },
  servers: [{ url: 'https://api.noobee.id/payment' }],
  tags: [{ name: 'refunds', description: 'Pengembalian dana' }, { name: 'payments' }],
  paths: {
    '/payments': {
      parameters: [{ name: 'X-Merchant', in: 'header', required: true, schema: { type: 'string' } }],
      get: {
        operationId: 'listPayments',
        summary: 'Daftar pembayaran',
        tags: ['payments'],
        parameters: [{ name: 'limit', in: 'query', schema: { type: 'integer', format: 'int32' } }],
        responses: {
          '200': {
            description: 'OK',
            content: {
              'application/json': {
                schema: { type: 'array', items: { $ref: '#/components/schemas/Payment' } },
                examples: { satu: { $ref: '#/components/examples/OnePayment' }, kosong: { summary: 'Tanpa data', value: [] } },
              },
            },
          },
          default: { description: 'Galat', content: { 'application/json': { schema: { $ref: '#/components/schemas/Problem' } } } },
        },
      },
      post: {
        operationId: 'createPayment',
        summary: 'Buat pembayaran',
        tags: ['payments'],
        security: [{ bearer: [] }],
        requestBody: {
          required: true,
          content: {
            'application/json': {
              schema: { $ref: '#/components/schemas/Payment' },
              example: { amount: 1500, note: null },
            },
            'application/xml': { schema: { $ref: '#/components/schemas/Payment' } },
          },
        },
        servers: [{ url: 'https://sandbox.noobee.id/payment' }],
        responses: { '201': { description: 'Dibuat' } },
      },
    },
  },
  components: {
    schemas: {
      Payment: {
        type: 'object',
        required: ['id', 'amount'],
        properties: {
          id: { type: 'string', format: 'uuid', readOnly: true },
          amount: { type: 'integer' },
          note: { type: 'string', nullable: true },
          parent: { $ref: '#/components/schemas/Payment' },
        },
      },
      Problem: { type: 'object', properties: { message: { type: 'string' } } },
    },
    examples: { OnePayment: { summary: 'Satu pembayaran', value: [{ id: '0b9a', amount: 1500 }] } },
    securitySchemes: {
      bearer: { type: 'http', scheme: 'bearer', bearerFormat: 'JWT', description: 'Token **bearer**' },
      apiKey: { type: 'apiKey', in: 'header', name: 'X-Api-Key' },
    },
  },
};

export const mockDetail = (over: Partial<ApiSpecDetail> = {}): ApiSpecDetail => ({
  spec: {
    name: 'payment',
    title: 'Payment API',
    openapi_version: '3.0.3',
    api_version: '1.4.0',
    format: 'yaml',
    source_path: 'specs/payment.yaml',
    size_bytes: 40211,
    endpoint_count: mockEndpoints.length,
    created_at: '2026-10-23T09:00:00Z',
    created_by_label: 'Budi',
    updated_at: '2026-10-24T10:00:00Z',
    updated_by_label: 'Sari',
  },
  endpoints: mockEndpoints,
  document: mockDocument,
  ...over,
});
