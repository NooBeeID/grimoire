// Spec bermuatan berbahaya: semua teks harus tampil sebagai teks (T06-AC8).
import { mockDetail } from './detail-3.0';
import { mockDetailSwagger2 } from './detail-swagger2';
import type { ApiSpecDetail, ApiSpecDetailWire, ApiSpecEndpoint } from '../types';

export const XSS_SCRIPT = '<script>alert(1)</script>';
export const XSS_IMG = '<img src=x onerror=alert(1)>';
export const XSS_LINK = '[klik](javascript:alert(1))';

export const mockEndpointsXss: ApiSpecEndpoint[] = [
  { method: 'GET', path: '/x', operation_id: 'x', summary: XSS_IMG, tags: ['evil'], deprecated: false },
];

export const mockDocumentXss = {
  openapi: '3.0.3',
  info: {
    title: 'Evil',
    version: '1',
    description: `${XSS_SCRIPT}\n\n${XSS_IMG}\n\n${XSS_LINK}\n\n[ok](https://docs.noobee.id/ok)\n\n![pelacak](https://tracker.example/p.png)`,
    termsOfService: 'javascript:alert(1)',
    contact: { name: 'x', url: 'javascript:alert(2)' },
    license: { name: 'x', url: 'javascript:alert(3)' },
  },
  servers: [{ url: 'javascript:alert(1)' }],
  externalDocs: { url: 'javascript:alert(1)', description: 'docs' },
  tags: [{ name: 'evil', description: `${XSS_IMG} ${XSS_LINK}` }],
  paths: {
    '/x': {
      get: {
        summary: XSS_IMG,
        description: `${XSS_SCRIPT}\n\n${XSS_LINK}`,
        tags: ['evil'],
        parameters: [{ name: 'q', in: 'query', description: `${XSS_IMG}\n\n${XSS_LINK}`, schema: { type: 'string' }, example: XSS_SCRIPT }],
        responses: {
          '200': {
            description: `${XSS_SCRIPT}`,
            content: { 'application/json': { schema: { $ref: '#/components/schemas/Evil' }, example: { a: XSS_IMG } } },
          },
        },
      },
    },
  },
  components: {
    schemas: { Evil: { type: 'object', description: `${XSS_IMG} ${XSS_LINK}`, properties: { a: { type: 'string', description: XSS_SCRIPT } } } },
    securitySchemes: { k: { type: 'apiKey', in: 'header', name: XSS_IMG, description: XSS_LINK } },
  },
};

export const mockDetailXss = (): ApiSpecDetail => mockDetail({ document: mockDocumentXss, endpoints: mockEndpointsXss });

export const mockDocumentSwagger2Xss = {
  swagger: '2.0' as const,
  info: { title: 'Evil', version: '1', description: `${XSS_SCRIPT}\n\n${XSS_IMG}\n\n${XSS_LINK}` },
  host: 'javascript:alert(1)',
  basePath: '/v1',
  schemes: ['https'],
  externalDocs: { url: 'javascript:alert(1)', description: 'docs' },
  tags: [{ name: 'evil' }],
  securityDefinitions: { k: { type: 'basic', description: `${XSS_IMG} ${XSS_LINK}` } },
  paths: {
    '/x': {
      post: {
        summary: XSS_IMG,
        tags: ['evil'],
        parameters: [{ name: 'body', in: 'body', description: `${XSS_IMG}\n\n${XSS_LINK}`, schema: { $ref: '#/definitions/Evil' } }],
        responses: { '200': { description: XSS_SCRIPT } },
      },
    },
  },
  definitions: { Evil: { type: 'object', description: XSS_IMG, properties: { a: { type: 'string', description: XSS_SCRIPT } } } },
};

export const mockDetailSwagger2Xss = (): ApiSpecDetailWire =>
  mockDetailSwagger2({
    document: mockDocumentSwagger2Xss,
    endpoints: [{ method: 'POST', path: '/x', operation_id: null, summary: XSS_IMG, tags: ['evil'], deprecated: false }],
  });
