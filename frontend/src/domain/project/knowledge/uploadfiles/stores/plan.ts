import { isHidden, joinPath } from '../../base';
import type { LocalFile, ReportItem, UploadPlan, UploadTarget } from '../types';

// Murni: hanya menyaring file tersembunyi dan tipe yang tidak ada targetnya; validasi path lain di backend.
export function planUpload(files: LocalFile[], targets: UploadTarget[], targetDir: string, hiddenReasonText = 'hidden', unsupportedText = 'unsupported'): UploadPlan {
  const plan: UploadPlan = { byTarget: new Map(), skipped: [], unsupported: [], totalBytes: 0 };
  for (const f of files) {
    const path = joinPath(targetDir, f.path);
    if (isHidden(path)) {
      plan.skipped.push({ path, status: 'skipped', reasonText: hiddenReasonText, target: 'client' } satisfies ReportItem);
      continue;
    }
    const target = targets.find((t) => t.accepts(path));
    if (!target) {
      plan.unsupported.push({ path, status: 'unsupported', reasonText: unsupportedText, target: 'client' });
      continue;
    }
    const list = plan.byTarget.get(target.key) ?? [];
    list.push({ path, file: f.file });
    plan.byTarget.set(target.key, list);
    plan.totalBytes += f.file.size;
  }
  return plan;
}
