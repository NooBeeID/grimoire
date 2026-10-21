// Data mock sesuai skema ApiSpecList (api/modules/project/api-spec.yml).
import type { ApiSpec, ApiSpecList } from '../types';

export const mockSpec = (over: Partial<ApiSpec> = {}): ApiSpec => ({
  name: 'payment',
  title: 'Payment API',
  openapi_version: '3.0.3',
  api_version: '1.4.0',
  format: 'yaml',
  source_path: 'specs/payment.yaml',
  size_bytes: 40211,
  endpoint_count: 2,
  created_at: '2026-10-23T09:00:00Z',
  created_by_label: 'Budi',
  updated_at: '2026-10-24T10:00:00Z',
  updated_by_label: 'Sari',
  ...over,
});

export const mockAuthSpec = (over: Partial<ApiSpec> = {}): ApiSpec =>
  mockSpec({
    name: 'auth',
    title: 'Auth API',
    openapi_version: '3.1.0',
    api_version: '2.0.1',
    source_path: 'specs/auth.yaml',
    size_bytes: 18234,
    endpoint_count: 5,
    created_by_label: 'Ana',
    updated_by_label: 'Ana',
    ...over,
  });

export const mockSpecList = (items: ApiSpec[] = [mockAuthSpec(), mockSpec()], limit = 100): ApiSpecList => ({
  items,
  total: items.length,
  limit,
});
