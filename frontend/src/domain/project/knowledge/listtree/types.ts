import { z } from 'zod';

export const FolderNodeSchema = z.object({ name: z.string(), path: z.string(), file_count: z.number().int().nonnegative() });
export const FileNodeSchema = z.object({
  name: z.string(),
  path: z.string(),
  size_bytes: z.number().int().nonnegative(),
  updated_at: z.string().datetime({ offset: true }),
});
export const TreeLevelSchema = z.object({
  prefix: z.string(),
  folders: z.array(FolderNodeSchema),
  files: z.array(FileNodeSchema),
  total_files: z.number().int().nonnegative(),
});
export type TreeLevel = z.infer<typeof TreeLevelSchema>;

export type TreeAction = 'open' | 'copyPath' | 'move' | 'deleteFile' | 'newFileHere' | 'uploadHere' | 'deleteFolder';
export interface TreeNodeRef {
  kind: 'folder' | 'file';
  path: string;
  name: string;
  fileCount?: number;
}
export interface TreeRow {
  key: string;
  kind: 'folder' | 'file' | 'loading' | 'error' | 'empty';
  path: string;
  name: string;
  depth: number;
  expanded?: boolean;
  fileCount?: number;
  parent: string;
}
