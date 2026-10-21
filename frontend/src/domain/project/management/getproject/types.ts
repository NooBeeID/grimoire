import { z } from 'zod';

// Salinan lokal bentuk ProjectDetail (api/modules/project/management.yml).
export const ProjectDetailSchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  description: z.string(),
  created_by: z.string().nullable(),
  created_by_label: z.string().nullable(),
  created_at: z.string().datetime({ offset: true }),
  updated_at: z.string().datetime({ offset: true }),
  my_role: z.enum(['owner', 'member']).nullable(),
  access_level: z.enum(['owner', 'member', 'admin']),
  is_member: z.boolean(),
  member_count: z.number().int().nonnegative(),
  counts: z.object({
    files: z.number().int().nonnegative().nullable(),
    specs: z.number().int().nonnegative().nullable(),
    keys: z.number().int().nonnegative().nullable(),
  }),
  owner_count: z.number().int().nonnegative(),
});
export type ProjectDetail = z.infer<typeof ProjectDetailSchema>;
export type AccessLevel = 'owner' | 'member' | 'admin';

export interface ProjectContext {
  project: ProjectDetail;
  reload(): Promise<void>; // GET ulang; dipakai setelah 403 atau perubahan peran sendiri
  replace(p: ProjectDetail): void; // pakai hasil PATCH tanpa GET ulang
}

// Hanya UX (rule 12.7); otorisasi sebenarnya di backend. Mencerminkan overview 3.4.
export function canManageMembers(l: AccessLevel): boolean {
  return l === 'owner' || l === 'admin';
}
export function canDeleteProject(l: AccessLevel): boolean {
  return l === 'owner' || l === 'admin';
}
