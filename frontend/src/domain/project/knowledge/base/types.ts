import { z } from 'zod';

// Kontrak: api/modules/project/knowledge.yml (info.version 0.1.0).
export const VERSION_RE = /^[0-9a-f]{64}$/;
const Iso = z.string().datetime({ offset: true });

export const UserRefSchema = z.object({ id: z.string(), label: z.string() });

export const KnowledgeFileSchema = z.object({
  path: z.string().min(1),
  title: z.string(),
  content: z.string(),
  size_bytes: z.number().int().nonnegative(),
  version: z.string().regex(VERSION_RE),
  created_at: Iso,
  created_by: UserRefSchema,
  updated_at: Iso,
  updated_by: UserRefSchema,
});
export type KnowledgeFile = z.infer<typeof KnowledgeFileSchema>;

export const ConflictCurrentSchema = z.object({
  content: z.string(),
  version: z.string().regex(VERSION_RE),
  updated_at: Iso,
  updated_by_label: z.string(),
});
export type ConflictCurrent = z.infer<typeof ConflictCurrentSchema>;
export const ConflictDetailsSchema = z.object({ reason: z.literal('version_conflict'), current: ConflictCurrentSchema });
export const TooLargeDetailsSchema = z.object({ reason: z.string(), max_bytes: z.number().int().positive() });

export const REASONS = [
  'path_empty', 'path_absolute', 'path_traversal', 'path_empty_segment', 'path_control_char',
  'path_too_long', 'segment_too_long', 'hidden', 'unsupported_type', 'too_large', 'invalid_encoding',
  'duplicate_in_request', 'too_many_files', 'project_file_limit', 'invalid_version', 'same_path',
  'prefix_required', 'invalid_limit', 'no_files', 'upload_too_large', 'path_exists', 'version_conflict', 'not_multipart',
] as const;
export type Reason = (typeof REASONS)[number];
