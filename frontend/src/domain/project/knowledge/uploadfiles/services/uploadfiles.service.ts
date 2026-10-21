import { postMultipart, type UploadProgress } from '../../../../../shared/services/upload';
import { get } from 'svelte/store';
import { t } from '../../../../../shared/stores/i18n';
import { reasonMessageKey } from '../../base';
import { UploadResultSchema, type LocalFile, type ReportItem, type UploadResult, type UploadTarget } from '../types';
import { isSupported } from '../../base';

export function uploadKnowledgeFiles(
  projectId: string,
  files: LocalFile[],
  o: { signal?: AbortSignal; onProgress?: (p: UploadProgress) => void } = {},
): Promise<UploadResult> {
  const form = new FormData();
  // Nama file part = path relatif; backend membaca filename mentah.
  for (const f of files) form.append('files', f.file, f.path);
  return postMultipart(`/api/projects/${encodeURIComponent(projectId)}/knowledge/upload`, form, { schema: UploadResultSchema, ...o });
}

export const knowledgeTarget: UploadTarget = {
  key: 'knowledge',
  labelKey: 'knowledge.upload.target.knowledge',
  accepts: isSupported,
  async upload(projectId, files, o) {
    const r = await uploadKnowledgeFiles(projectId, files, o);
    const tr = get(t);
    return r.items.map<ReportItem>((i) => {
      const key = reasonMessageKey(i.reason ?? null);
      return { path: i.path, status: i.status, reasonText: key ? tr(key, { max: "" }).trim() : i.reason, target: 'knowledge' };
    });
  },
};
