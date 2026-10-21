// Data mock sesuai skema ApiSpecUploadResult / ApiSpecValidateResult (api/modules/project/api-spec.yml).
import type { ApiSpecFileResult, ApiSpecUploadResult, ApiSpecValidateResult } from '../types';

export const mockFileResult = (over: Partial<ApiSpecFileResult> = {}): ApiSpecFileResult => ({
  file: 'payment.yaml',
  source_path: 'specs/payment.yaml',
  name: 'payment',
  status: 'created',
  reason_code: null,
  reason: null,
  issues: [],
  openapi_version: '3.0.3',
  title: 'Payment API',
  api_version: '1.4.0',
  endpoint_count: 2,
  ...over,
});

export const mockRejected = (over: Partial<ApiSpecFileResult> = {}): ApiSpecFileResult =>
  mockFileResult({
    file: 'legacy.yaml',
    source_path: 'legacy.yaml',
    name: 'legacy',
    status: 'rejected',
    reason_code: 'INVALID_SPEC',
    reason: 'Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1',
    issues: [
      {
        code: 'NOT_OPENAPI_3',
        message: 'Swagger 2.0 tidak didukung, gunakan OpenAPI 3.0/3.1 (baris 1, #/swagger)',
        pointer: '#/swagger',
        line: 1,
      },
    ],
    openapi_version: null,
    title: null,
    api_version: null,
    endpoint_count: null,
    ...over,
  });

export const mockUploadResult = (results: ApiSpecFileResult[] = [mockFileResult(), mockRejected()]): ApiSpecUploadResult => ({
  results,
  summary: {
    created: results.filter((r) => r.status === 'created').length,
    updated: results.filter((r) => r.status === 'updated').length,
    unchanged: results.filter((r) => r.status === 'unchanged').length,
    skipped: results.filter((r) => r.status === 'skipped').length,
    rejected: results.filter((r) => r.status === 'rejected').length,
  },
});

export const mockValidateResult = (results: ApiSpecFileResult[]): ApiSpecValidateResult => ({
  results,
  summary: {
    valid: results.filter((r) => r.status === 'valid').length,
    skipped: results.filter((r) => r.status === 'skipped').length,
    rejected: results.filter((r) => r.status === 'rejected').length,
  },
});
