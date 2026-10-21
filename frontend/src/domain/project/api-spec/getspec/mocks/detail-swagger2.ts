// Data mock Swagger 2.0 (bentuk kawat GET detail; dokumen belum diadaptasi).
import type { ApiSpecDetailWire, ApiSpecEndpoint } from '../types';

export const mockEndpointsSwagger2: ApiSpecEndpoint[] = [
  { method: 'GET', path: '/employees/{employeeId}', operation_id: 'getEmployee', summary: 'Detail karyawan', tags: ['employees'], deprecated: false },
  { method: 'POST', path: '/employees', operation_id: 'createEmployee', summary: 'Tambah karyawan', tags: ['employees'], deprecated: false },
  { method: 'POST', path: '/employees/{employeeId}/documents', operation_id: 'uploadEmployeeDocument', summary: 'Unggah dokumen karyawan', tags: ['employees'], deprecated: false },
  { method: 'GET', path: '/leave-requests', operation_id: 'listLeaveRequests', summary: 'Daftar pengajuan cuti', tags: ['leave'], deprecated: true },
  { method: 'POST', path: '/leave-requests/{leaveRequestId}/approve', operation_id: 'approveLeaveRequest', summary: 'Setujui pengajuan cuti', tags: ['leave'], deprecated: false },
];

export const mockDocumentSwagger2 = {
  swagger: '2.0' as const,
  info: { title: 'HR Platform API', version: '1.12.0', description: 'Data karyawan dan cuti.' },
  host: 'api.hr-platform.noobee.id',
  basePath: '/v1',
  schemes: ['https'],
  consumes: ['application/json'],
  produces: ['application/json'],
  tags: [{ name: 'employees', description: 'Data karyawan' }, { name: 'leave', description: 'Cuti' }],
  securityDefinitions: {
    bearerAuth: { type: 'apiKey', in: 'header', name: 'Authorization' },
    basicAuth: { type: 'basic' },
    hrOauth: {
      type: 'oauth2',
      flow: 'accessCode',
      authorizationUrl: 'https://auth.hr-platform.noobee.id/oauth/authorize',
      tokenUrl: 'https://auth.hr-platform.noobee.id/oauth/token',
      scopes: { 'employees:read': 'Baca karyawan', 'employees:write': 'Ubah karyawan' },
    },
  },
  security: [{ hrOauth: ['employees:read'] }],
  parameters: {
    TenantHeader: { name: 'X-Tenant-ID', in: 'header', required: true, type: 'string' },
  },
  responses: {
    NotFound: {
      description: 'Tidak ditemukan',
      schema: { $ref: '#/definitions/Error' },
      examples: { 'application/json': { code: 'EMPLOYEE_NOT_FOUND', message: 'Karyawan EMP-00412 tidak ditemukan' } },
    },
  },
  paths: {
    '/employees/{employeeId}': {
      parameters: [{ name: 'employeeId', in: 'path', required: true, type: 'string' }],
      get: {
        operationId: 'getEmployee',
        summary: 'Detail karyawan',
        tags: ['employees'],
        responses: {
          '200': { description: 'OK', schema: { $ref: '#/definitions/Employee' } },
          '404': { $ref: '#/responses/NotFound' },
        },
      },
    },
    '/employees': {
      post: {
        operationId: 'createEmployee',
        summary: 'Tambah karyawan',
        tags: ['employees'],
        security: [{ hrOauth: ['employees:write'] }],
        parameters: [{ name: 'body', in: 'body', required: true, schema: { $ref: '#/definitions/Employee' } }],
        responses: {
          '201': { description: 'Karyawan dibuat', schema: { $ref: '#/definitions/Employee' } },
          '400': { description: 'Input tidak valid', schema: { $ref: '#/definitions/Error' } },
        },
      },
    },
    '/employees/{employeeId}/documents': {
      parameters: [{ name: 'employeeId', in: 'path', required: true, type: 'string' }],
      post: {
        operationId: 'uploadEmployeeDocument',
        summary: 'Unggah dokumen karyawan',
        tags: ['employees'],
        consumes: ['multipart/form-data'],
        parameters: [
          { name: 'file', in: 'formData', required: true, type: 'file' },
          { name: 'category', in: 'formData', type: 'string', enum: ['ktp', 'npwp', 'kontrak'] },
        ],
        responses: { '201': { description: 'Dokumen tersimpan' }, '413': { description: 'Berkas terlalu besar', schema: { $ref: '#/definitions/Error' } } },
      },
    },
    '/leave-requests': {
      parameters: [{ $ref: '#/parameters/TenantHeader' }],
      get: {
        operationId: 'listLeaveRequests',
        summary: 'Daftar pengajuan cuti',
        tags: ['leave'],
        deprecated: true,
        parameters: [
          { name: 'status', in: 'query', type: 'array', items: { type: 'string', enum: ['pending', 'approved', 'rejected'] }, collectionFormat: 'csv' },
        ],
        responses: { '200': { description: 'OK', schema: { type: 'array', items: { $ref: '#/definitions/Employee' } } } },
      },
    },
    '/leave-requests/{leaveRequestId}/approve': {
      parameters: [{ name: 'leaveRequestId', in: 'path', required: true, type: 'string' }],
      post: {
        operationId: 'approveLeaveRequest',
        summary: 'Setujui pengajuan cuti',
        tags: ['leave'],
        security: [{ basicAuth: [] }],
        responses: { '204': { description: 'Disetujui' } },
      },
    },
  },
  definitions: {
    Person: {
      type: 'object',
      discriminator: 'kind',
      required: ['kind', 'fullName'],
      properties: { kind: { type: 'string' }, fullName: { type: 'string' } },
    },
    OrgUnit: {
      type: 'object',
      properties: { code: { type: 'string' }, parent: { $ref: '#/definitions/OrgUnit' } },
    },
    Employee: {
      allOf: [
        { $ref: '#/definitions/Person' },
        {
          type: 'object',
          required: ['employeeNo'],
          properties: {
            employeeNo: { type: 'string', example: 'EMP-00412' },
            orgUnit: { $ref: '#/definitions/OrgUnit' },
            terminatedAt: { type: 'string', format: 'date-time', 'x-nullable': true },
          },
        },
      ],
    },
    Error: { type: 'object', properties: { code: { type: 'string' }, message: { type: 'string' } } },
  },
};

export const mockDetailSwagger2 = (over: Partial<ApiSpecDetailWire> = {}): ApiSpecDetailWire => ({
  spec: {
    name: 'hr-platform',
    title: 'HR Platform API',
    openapi_version: '2.0',
    api_version: '1.12.0',
    format: 'yaml',
    source_path: 'api/openapi.yml',
    size_bytes: 18432,
    endpoint_count: mockEndpointsSwagger2.length,
    created_at: '2026-10-21T09:00:00Z',
    created_by_label: 'Rina Pratiwi',
    updated_at: '2026-10-21T09:00:00Z',
    updated_by_label: 'Rina Pratiwi',
  },
  endpoints: mockEndpointsSwagger2,
  document: mockDocumentSwagger2,
  ...over,
});
