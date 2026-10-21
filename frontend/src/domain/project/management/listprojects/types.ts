import { z } from 'zod';

export const PAGE_SIZE = 20;
export const MAX_QUERY = 100;
export const MY_ROLES = ['owner', 'member'] as const;
export const ACCESS_LEVELS = ['owner', 'member', 'admin'] as const;

// Mengikuti ProjectSummary/ProjectList di api/modules/project/management.yml.
export const ContentCountsSchema = z.object({
  files: z.number().int().nonnegative().nullable(),
  specs: z.number().int().nonnegative().nullable(),
  keys: z.number().int().nonnegative().nullable(),
});
export const ProjectSummarySchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  description: z.string(),
  created_by: z.string().nullable(),
  created_by_label: z.string().nullable(),
  created_at: z.string().datetime({ offset: true }),
  updated_at: z.string().datetime({ offset: true }),
  my_role: z.enum(MY_ROLES).nullable(),
  access_level: z.enum(ACCESS_LEVELS),
  is_member: z.boolean(),
  member_count: z.number().int().nonnegative(),
  counts: ContentCountsSchema,
});
export const ProjectListSchema = z.object({
  items: z.array(ProjectSummarySchema),
  total: z.number().int().nonnegative(),
  limit: z.number().int().positive(),
  offset: z.number().int().nonnegative(),
});
export type ProjectSummary = z.infer<typeof ProjectSummarySchema>;
export type ProjectList = z.infer<typeof ProjectListSchema>;
export interface ProjectListQuery {
  q: string;
  page: number; // >= 1
}
