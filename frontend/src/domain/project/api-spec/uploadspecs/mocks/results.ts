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
    reason: 'versi Swagger "1.2" tidak didukung (hanya 2.0)',
    issues: [
      {
        code: 'UNSUPPORTED_VERSION',
        message: 'versi Swagger "1.2" tidak didukung (hanya 2.0) (baris 1, #/swagger)',
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

export const mockSwagger2Created = (over: Partial<ApiSpecFileResult> = {}): ApiSpecFileResult =>
  mockFileResult({
    file: 'petstore.yaml',
    source_path: 'specs/petstore.yaml',
    name: 'petstore',
    openapi_version: '2.0',
    title: 'Swagger Petstore',
    api_version: '1.0.0',
    endpoint_count: 4,
    ...over,
  });

export const mockSwagger2Unresolved = (over: Partial<ApiSpecFileResult> = {}): ApiSpecFileResult =>
  mockRejected({
    file: 'broken.yaml',
    source_path: 'broken.yaml',
    name: 'broken',
    reason: 'referensi tidak ditemukan',
    issues: [
      {
        code: 'UNRESOLVED_REF',
        message: 'referensi "#/definitions/Hilang" tidak ditemukan (baris 21, #/paths/~1pets/get/responses/200/schema/$ref)',
        pointer: '#/paths/~1pets/get/responses/200/schema/$ref',
        line: 21,
      },
    ],
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
