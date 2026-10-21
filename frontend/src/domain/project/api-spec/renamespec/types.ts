import { z } from 'zod';

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
export const RenameResponseSchema = z.object({ spec: ApiSpecSchema });
export type ApiSpec = z.infer<typeof ApiSpecSchema>;
export const NAME_RE = /^[a-z0-9._-]{1,80}$/;

export function validateName(v: string): 'ok' | 'required' | 'invalid' {
  if (v.trim() === '') return 'required';
  return NAME_RE.test(v) && v !== '.' && v !== '..' ? 'ok' : 'invalid';
}
