import type { GitSyncReportItem, Kind } from './types';

type Translate = (key: string, params?: Record<string, string | number>) => string;

export const reasonKey = (kind: Kind, code: string): string => (kind === 'knowledge' ? `knowledge.reason.${code}` : `apispec.reason.${code}`);
export const errorKey = (code: string): string => `gitSync.errors.${code}`;

// Terjemahan bila kunci ada; bila tidak (t mengembalikan kuncinya), pakai teks server, lalu kode mentah.
export function reasonText(t: Translate, kind: Kind, item: Pick<GitSyncReportItem, 'reason_code' | 'reason'>): string {
  if (item.reason_code === '') return item.reason;
  const key = reasonKey(kind, item.reason_code);
  const tr = t(key, { max: t('gitSync.reason.limitUnknown') });
  if (tr !== key) return tr;
  return item.reason !== '' ? item.reason : item.reason_code;
}

export function errorText(t: Translate, code: string, fallback: string): string {
  if (code === '') return fallback;
  const key = errorKey(code);
  const tr = t(key);
  return tr !== key ? tr : fallback !== '' ? fallback : code;
}
