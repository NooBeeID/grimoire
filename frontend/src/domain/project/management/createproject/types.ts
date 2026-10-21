import { z } from 'zod';

export const NAME_MIN = 2;
export const NAME_MAX = 100;
export const DESCRIPTION_MAX = 1000;

export interface CreateProjectInput {
  name: string;
  description: string;
}

// Salinan lokal bentuk ProjectSummary (feature tidak meng-import internals feature lain).
export const CreatedProjectSchema = z.object({
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
});
export type CreatedProject = z.infer<typeof CreatedProjectSchema>;
