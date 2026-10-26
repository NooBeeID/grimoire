import { z } from 'zod';

export const USAGE_RANGES = ['24h', '7d', '30d'] as const;
export type UsageRange = (typeof USAGE_RANGES)[number];
export const DEFAULT_USAGE_RANGE: UsageRange = '7d';

export const USAGE_SECTIONS = ['top_files', 'unread_files', 'zero_queries', 'keys'] as const;
export type UsageSection = (typeof USAGE_SECTIONS)[number];
// Nilai parameter `keys` dijawab pada kunci respons `per_key`.
export const SECTION_KEY = {
  top_files: 'top_files',
  unread_files: 'unread_files',
  zero_queries: 'zero_queries',
  keys: 'per_key',
} as const;
export const PROJECT_SECTIONS: readonly UsageSection[] = ['top_files', 'unread_files'];

export interface UsageFilter {
  range: UsageRange;
  projectId: string | null;
}

const count = z.number().int().min(0);
const at = z.string().datetime({ offset: true });
const errorCode = z.enum(['UNAVAILABLE', 'PROJECT_REQUIRED']).optional();

export const FileReadSchema = z
  .object({ path: z.string(), reads: count, keys: count, last_read_at: at })
  .strict();
export const UnreadFileSchema = z.object({ path: z.string(), updated_at: at }).strict();
export const ZeroQuerySchema = z.object({ query: z.string(), count, keys: count, last_at: at }).strict();
export const KeyUsageSchema = z
  .object({
    key_id: z.string(),
    label: z.string(),
    calls: count,
    errors: count,
    searches: count,
    reads: count,
    last_used_at: at,
    top_tool: z.string(),
  })
  .strict();

const section = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ available: z.boolean(), error_code: errorCode, items: z.array(item) }).strict();

export const AgentUsageSchema = z
  .object({
    range: z.enum(USAGE_RANGES),
    from: at,
    to: at,
    project_id: z.string().uuid().nullable(),
    top_files: section(FileReadSchema).nullable(),
    unread_files: section(UnreadFileSchema).extend({ truncated: z.boolean() }).strict().nullable(),
    zero_queries: section(ZeroQuerySchema).nullable(),
    per_key: section(KeyUsageSchema).nullable(),
  })
  .strict();

export type AgentUsage = z.infer<typeof AgentUsageSchema>;
export type FileRead = z.infer<typeof FileReadSchema>;
export type UnreadFile = z.infer<typeof UnreadFileSchema>;
export type ZeroQuery = z.infer<typeof ZeroQuerySchema>;
export type KeyUsage = z.infer<typeof KeyUsageSchema>;
