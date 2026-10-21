import { z } from 'zod';

// Mengikuti ApiSpecFileResult / ApiSpecUploadResult / ApiSpecValidateResult di api/modules/project/api-spec.yml.
export const FILE_STATUSES = ['created', 'updated', 'unchanged', 'skipped', 'rejected', 'valid'] as const;
export const FileStatusSchema = z.enum(FILE_STATUSES);
export type FileStatus = z.infer<typeof FileStatusSchema>;

export const ApiSpecIssueSchema = z.object({
  code: z.string(),
  message: z.string(),
  pointer: z.string().nullable(),
  line: z.number().int().positive().nullable(),
});
export const ApiSpecFileResultSchema = z.object({
  file: z.string(),
  source_path: z.string().nullable(),
  name: z.string().nullable(),
  status: FileStatusSchema,
  reason_code: z.string().nullable(),
  reason: z.string().nullable(),
  issues: z.array(ApiSpecIssueSchema),
  openapi_version: z.string().nullable(),
  title: z.string().nullable(),
  api_version: z.string().nullable(),
  endpoint_count: z.number().int().nonnegative().nullable(),
});
export const ApiSpecUploadResultSchema = z.object({
  results: z.array(ApiSpecFileResultSchema),
  summary: z.object({
    created: z.number().int(),
    updated: z.number().int(),
    unchanged: z.number().int(),
    skipped: z.number().int(),
    rejected: z.number().int(),
  }),
});
export const ApiSpecValidateResultSchema = z.object({
  results: z.array(ApiSpecFileResultSchema),
  summary: z.object({ valid: z.number().int(), skipped: z.number().int(), rejected: z.number().int() }),
});
export type ApiSpecIssue = z.infer<typeof ApiSpecIssueSchema>;
export type ApiSpecFileResult = z.infer<typeof ApiSpecFileResultSchema>;
export type ApiSpecUploadResult = z.infer<typeof ApiSpecUploadResultSchema>;
export type ApiSpecValidateResult = z.infer<typeof ApiSpecValidateResultSchema>;

export interface PickedFile {
  file: File;
  path: string; // webkitRelativePath || name
}
export const SPEC_EXTENSIONS = ['.yaml', '.yml', '.json'] as const;
export const NAME_RE = /^[a-z0-9._-]{1,80}$/;
