import { z } from 'zod';
import type { UploadProgress } from '../../../../shared/services/upload';

export const UploadStatusSchema = z.enum(['created', 'updated', 'unchanged', 'skipped', 'rejected']);
export const UploadItemSchema = z.object({ path: z.string(), status: UploadStatusSchema, reason: z.string().optional() });
const Count = z.number().int().nonnegative();
export const UploadResultSchema = z.object({
  summary: z.object({ created: Count, updated: Count, unchanged: Count, skipped: Count, rejected: Count }),
  items: z.array(UploadItemSchema),
});
export type UploadResult = z.infer<typeof UploadResultSchema>;

export interface LocalFile {
  path: string; // relatif (webkitRelativePath)
  file: File;
}
export type ReportStatus = z.infer<typeof UploadStatusSchema> | 'unsupported';
export interface ReportItem {
  path: string;
  status: ReportStatus;
  reasonText?: string;
  target: string;
}
// Titik ekstensi untuk upload campuran; app/ mendaftarkan target tambahan (mis. api-spec T06).
export interface UploadTarget {
  key: string;
  labelKey: string;
  // Ekstensi yang diterima target ini (mis. ['.yaml', '.json']); dipakai untuk atribut accept pemilih file.
  acceptExts?: string[];
  accepts(path: string): boolean;
  upload(projectId: string, files: LocalFile[], o: { signal: AbortSignal; onProgress(p: UploadProgress): void }): Promise<ReportItem[]>;
}
export interface UploadPlan {
  byTarget: Map<string, LocalFile[]>;
  skipped: ReportItem[];
  unsupported: ReportItem[];
  totalBytes: number;
}
