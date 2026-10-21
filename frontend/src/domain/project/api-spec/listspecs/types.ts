import { z } from 'zod';

// Mengikuti ApiSpec / ApiSpecList di api/modules/project/api-spec.yml.
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
export const ApiSpecListSchema = z.object({
  items: z.array(ApiSpecSchema),
  total: z.number().int().nonnegative(),
  limit: z.number().int().positive(),
});
export type ApiSpec = z.infer<typeof ApiSpecSchema>;
export type ApiSpecList = z.infer<typeof ApiSpecListSchema>;

// Cermin ORDER BY name backend (perbandingan kode unit). Mengembalikan salinan.
export function sortSpecs(xs: ApiSpec[]): ApiSpec[] {
  return [...xs].sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
}

// "3.1.0" -> "3.1"; selain itu dikembalikan apa adanya.
export function openapiMajorMinor(v: string): string {
  const m = /^(\d+\.\d+)(\.|$)/.exec(v);
  return m ? m[1]! : v;
}

export function formatBytes(n: number, locale: 'id' | 'en'): string {
  const nf = (v: number) => new Intl.NumberFormat(locale === 'id' ? 'id-ID' : 'en-US', { maximumFractionDigits: 1, minimumFractionDigits: v < 10 && n >= 1024 ? 1 : 0 }).format(v);
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${nf(n / 1024)} KB`;
  return `${nf(n / (1024 * 1024))} MB`;
}
