import { z } from 'zod';

// Mengikuti ApiKey / ApiKeyList di api/modules/project/api-key.yml.
export const KEY_STATUSES = ['active', 'revoked'] as const;
export const ApiKeySchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  prefix: z.string().regex(/^[a-z0-9]{8}$/),
  status: z.enum(KEY_STATUSES),
  created_by: z.string().nullable(),
  created_by_label: z.string().nullable(),
  created_at: z.string().datetime({ offset: true }),
  last_used_at: z.string().datetime({ offset: true }).nullable(),
  revoked_at: z.string().datetime({ offset: true }).nullable(),
  revoked_by: z.string().nullable(),
  revoked_by_label: z.string().nullable(),
});
export const ApiKeyListSchema = z.object({ items: z.array(ApiKeySchema) });
export type ApiKey = z.infer<typeof ApiKeySchema>;
export type ApiKeyList = z.infer<typeof ApiKeyListSchema>;

// Cermin ORDER BY backend: aktif dulu, created_at DESC, id ASC. Mengembalikan salinan.
export function sortKeys(items: ApiKey[]): ApiKey[] {
  return [...items].sort((a, b) => {
    if (a.status !== b.status) return a.status === 'active' ? -1 : 1;
    const ta = Date.parse(a.created_at);
    const tb = Date.parse(b.created_at);
    if (ta !== tb) return tb - ta;
    return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
  });
}

export function displayPrefix(prefix: string): string {
  return `gr_${prefix}_…`;
}
