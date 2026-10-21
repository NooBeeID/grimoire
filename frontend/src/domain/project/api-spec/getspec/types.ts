import { z } from 'zod';

// Mengikuti ApiSpecDetail di api/modules/project/api-spec.yml. Salinan lokal ApiSpec (satu feature tidak mengimpor feature lain).
export const ApiSpecSchema = z.object({
  name: z.string().min(1),
  title: z.string(),
  openapi_version: z.string(),
  api_version: z.string(),
  format: z.enum(['yaml', 'json']),
  source_path: z.string().nullable(),
  size_bytes: z.number().int().nonnegative(),
  endpoint_count: z.number().int().nonnegative(),
  created_at: z.string().datetime({ offset: true }),
  created_by_label: z.string().nullable(),
  updated_at: z.string().datetime({ offset: true }),
  updated_by_label: z.string().nullable(),
});
export const HTTP_METHODS = ['GET', 'PUT', 'POST', 'DELETE', 'OPTIONS', 'HEAD', 'PATCH', 'TRACE'] as const;
export const ApiSpecEndpointSchema = z.object({
  method: z.string().min(1),
  path: z.string(),
  operation_id: z.string().nullable(),
  summary: z.string(),
  tags: z.array(z.string()),
  deprecated: z.boolean(),
});
// Isi dokumen di luar level atas bebas bentuk; model membacanya secara defensif.
export const OpenApiDocumentSchema = z.object({ openapi: z.string(), info: z.object({ title: z.string(), version: z.string() }).passthrough() }).passthrough();
export const ApiSpecDetailSchema = z.object({
  spec: ApiSpecSchema,
  endpoints: z.array(ApiSpecEndpointSchema),
  document: OpenApiDocumentSchema,
});
export type ApiSpec = z.infer<typeof ApiSpecSchema>;
export type ApiSpecEndpoint = z.infer<typeof ApiSpecEndpointSchema>;
export type ApiSpecDetail = z.infer<typeof ApiSpecDetailSchema>;
export type OpenApiDocument = z.infer<typeof OpenApiDocumentSchema>;
