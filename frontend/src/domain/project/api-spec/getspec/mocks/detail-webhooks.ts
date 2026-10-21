// Spec 3.1 hanya dengan webhooks (tanpa paths).
import { mockDetail } from './detail-3.0';
import type { ApiSpecDetail } from '../types';

export const mockDocumentWebhooks = {
  openapi: '3.1.0',
  info: { title: 'Events API', version: '1' },
  webhooks: {
    paymentCreated: {
      post: {
        summary: 'Pembayaran dibuat',
        requestBody: { content: { 'application/json': { schema: { type: 'object', properties: { id: { type: 'string' } } } } } },
        responses: { '200': { description: 'Diterima' } },
      },
    },
  },
};

export const mockDetailWebhooks = (): ApiSpecDetail => ({
  ...mockDetail({ document: mockDocumentWebhooks, endpoints: [] }),
  spec: { ...mockDetail().spec, name: 'events', title: 'Events API', openapi_version: '3.1.0', api_version: '1', endpoint_count: 0 },
});
