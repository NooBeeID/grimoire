import { z } from 'zod';

// Mengikuti SynonymGroup / SynonymGroupList di api/modules/project/search-synonym.yml.
export const SynonymGroupSchema = z.object({
  id: z.string().uuid(),
  terms: z.array(z.string()).min(2).max(10),
  created_at: z.string().datetime({ offset: true }),
  updated_at: z.string().datetime({ offset: true }),
  created_by_label: z.string(),
  updated_by_label: z.string(),
});
export const SynonymGroupListSchema = z.object({ items: z.array(SynonymGroupSchema) });
export type SynonymGroup = z.infer<typeof SynonymGroupSchema>;
export type SynonymGroupList = z.infer<typeof SynonymGroupListSchema>;

// Cermin ORDER BY backend: created_at ASC, id ASC. Mengembalikan salinan.
export function sortGroups(items: SynonymGroup[]): SynonymGroup[] {
  return [...items].sort((a, b) => {
    const ta = Date.parse(a.created_at);
    const tb = Date.parse(b.created_at);
    if (ta !== tb) return ta - tb;
    return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
  });
}
